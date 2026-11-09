package engines

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/arcvpn/arcvpn/internal/devices"
	"github.com/arcvpn/arcvpn/internal/protocols"
	"github.com/arcvpn/arcvpn/internal/store"
)

// Actual Hysteria QUIC/TLS over loopback, trusted fixture CA, no insecure TLS.
// Does not exercise systemd/root helper, public VPS/ACME or external reachability.
func TestRealHysteriaAccountingAndRevokedDevice(t *testing.T) {
	binary := os.Getenv("ARCVPN_TEST_HYSTERIA")
	if binary == "" {
		t.Skip("set ARCVPN_TEST_HYSTERIA to the verified pinned binary")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	certificate := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "hysteria.fixture"}, DNSNames: []string{"hysteria.fixture"}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	cert, err := x509.CreateCertificate(rand.Reader, certificate, certificate, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certPath, keyPath := filepath.Join(dir, "ca.pem"), filepath.Join(dir, "key.pem")
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert}), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0600); err != nil {
		t.Fatal(err)
	}
	udp, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	serverPort := udp.LocalAddr().(*net.UDPAddr).Port
	udp.Close()
	freeTCP := func() int {
		l, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		p := l.Addr().(*net.TCPAddr).Port
		l.Close()
		return p
	}
	d := devices.Device{ID: "22222222-2222-4222-8222-222222222222", UUID: "33333333-3333-4333-8333-333333333333"}
	users := []store.User{{ID: 1, UUID: "11111111-1111-4111-8111-111111111111", Enabled: true, DeviceAccess: &devices.Record{Devices: []devices.Device{d}}}, {ID: 2, UUID: "44444444-4444-4444-8444-444444444444", Enabled: true}}
	creds := []store.ProtocolCredential{{UserID: 1, Kind: protocols.Hysteria2, Secret: strings.Repeat("a", 43)}, {UserID: 2, Kind: protocols.Hysteria2, Secret: strings.Repeat("b", 43)}}
	m := Manager{Dir: dir}
	serverPath := filepath.Join(dir, "hysteria.yaml")
	writeServer := func() {
		t.Helper()
		p := store.Protocol{Kind: protocols.Hysteria2, Address: "127.0.0.1", SNI: "127.0.0.1", Port: serverPort, TLSCert: "/etc/arcvpn/tls/fullchain.pem", TLSKey: "/etc/arcvpn/tls/privkey.pem"}
		data, err := protocols.Render(p, users, creds)
		if err != nil {
			t.Fatal(err)
		}
		data, err = m.withHysteriaStatistics(data)
		if err != nil {
			t.Fatal(err)
		}
		var config map[string]any
		if err := json.Unmarshal(data, &config); err != nil {
			t.Fatal(err)
		}
		// Test-only paths/listen, preserving production allowlisted certificate paths.
		config["listen"] = "127.0.0.1:" + strconv.Itoa(serverPort)
		config["tls"] = map[string]string{"cert": certPath, "key": keyPath, "sniGuard": "dns-san"}
		data, _ = json.Marshal(config)
		if err := os.WriteFile(serverPath, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	writeServer()
	serverCmd := exec.Command(binary, "server", "-c", serverPath)
	serverLog, err := os.OpenFile(filepath.Join(dir, "server.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer serverLog.Close()
	serverCmd.Stdout = serverLog
	serverCmd.Stderr = serverLog
	if err := serverCmd.Start(); err != nil {
		t.Fatal(err)
	}
	stopServer := func() {
		if serverCmd != nil {
			_ = serverCmd.Process.Signal(syscall.SIGTERM)
			_ = serverCmd.Wait()
			serverCmd = nil
		}
	}
	defer stopServer()
	process := hysteriaProcess{PID: serverCmd.Process.Pid, Epoch: "first", StartedAt: time.Now().UnixNano()}
	waitStats := func(p hysteriaProcess) {
		t.Helper()
		for i := 0; i < 50; i++ {
			if _, _, err := m.hysteriaStatistics(context.Background(), users, creds, p); err == nil {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Fatal("real Hysteria statistics did not start")
	}
	waitStats(process)
	makeClient := func(authID, secret string) *http.Client {
		t.Helper()
		port := freeTCP()
		path := filepath.Join(dir, authID+"-client.json")
		config := map[string]any{"server": "127.0.0.1:" + strconv.Itoa(serverPort), "auth": authID + ":" + secret, "tls": map[string]any{"sni": "hysteria.fixture", "ca": certPath}, "socks5": map[string]any{"listen": "127.0.0.1:" + strconv.Itoa(port)}}
		blob, _ := json.Marshal(config)
		if err := os.WriteFile(path, blob, 0600); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(binary, "client", "-c", path)
		log, err := os.OpenFile(path+".log", os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			t.Fatal(err)
		}
		cmd.Stdout = log
		cmd.Stderr = log
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = cmd.Process.Signal(syscall.SIGTERM); _ = cmd.Wait(); log.Close() })
		proxy, _ := url.Parse("socks5://127.0.0.1:" + strconv.Itoa(port))
		transport := &http.Transport{Proxy: http.ProxyURL(proxy), DisableKeepAlives: true}
		t.Cleanup(transport.CloseIdleConnections)
		return &http.Client{Transport: transport, Timeout: time.Second}
	}
	payload := bytes.Repeat([]byte("x"), 128<<10)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(payload) }))
	defer target.Close()
	transfer := func(client *http.Client) error {
		resp, err := client.Get(target.URL)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		got, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		if !bytes.Equal(got, payload) {
			return io.ErrUnexpectedEOF
		}
		return nil
	}
	deviceClient := makeClient(protocols.HysteriaDeviceName(d.ID), d.UUID)
	transferred := false
	for i := 0; i < 40; i++ {
		if transfer(deviceClient) == nil {
			transferred = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !transferred {
		t.Fatal("real Hysteria client did not transfer fixture body")
	}
	counters, _, err := m.hysteriaStatistics(context.Background(), users, creds, process)
	if err != nil || counters[0].Upload == 0 || counters[0].Download < int64(len(payload)) {
		t.Fatal("real traffic not charged", counters, err)
	}
	again, _, err := m.hysteriaStatistics(context.Background(), users, creds, process)
	if err != nil || again[0].Download < counters[0].Download {
		t.Fatal("query cleared counters", err)
	}
	t.Logf("real QUIC/TLS counters: upload=%d download=%d", counters[0].Upload, counters[0].Download)
	// Apply a revoked allowlist through a real restart. The allowed control
	// credential proves service health while the revoked credential fails.
	stopServer()
	users[0].DeviceAccess.Devices[0].RevokedAt = time.Now().Unix()
	writeServer()
	serverCmd = exec.Command(binary, "server", "-c", serverPath)
	serverCmd.Stdout = serverLog
	serverCmd.Stderr = serverLog
	if err := serverCmd.Start(); err != nil {
		t.Fatal(err)
	}
	process = hysteriaProcess{PID: serverCmd.Process.Pid, Epoch: "second", StartedAt: time.Now().UnixNano()}
	waitStats(process)
	control := makeClient("u2", creds[1].Secret)
	healthy := false
	for i := 0; i < 40; i++ {
		if transfer(control) == nil {
			healthy = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !healthy {
		t.Fatal("post-restart control credential failed")
	}
	denied := makeClient(protocols.HysteriaDeviceName(d.ID), d.UUID)
	for i := 0; i < 3; i++ {
		if transfer(denied) == nil {
			t.Fatal("revoked device authenticated")
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Log("revoked credential denied after actual restart; control credential passed; no VPS/systemd acceptance")
	// Regression: disabling/deleting the final user must keep the real daemon
	// healthy while every previously issued credential is rejected.
	stopServer()
	users, creds = nil, nil
	writeServer()
	serverCmd = exec.Command(binary, "server", "-c", serverPath)
	serverCmd.Stdout, serverCmd.Stderr = serverLog, serverLog
	if err := serverCmd.Start(); err != nil {
		t.Fatal(err)
	}
	process = hysteriaProcess{PID: serverCmd.Process.Pid, Epoch: "empty", StartedAt: time.Now().UnixNano()}
	waitStats(process)
	if transfer(makeClient("u2", strings.Repeat("b", 43))) == nil {
		t.Fatal("previous credential accepted with empty roster")
	}
	t.Log("empty roster: real daemon and Stats API healthy; previously valid credential rejected")
}
