package engines

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"github.com/arcvpn/arcvpn/internal/protocols"
	"github.com/arcvpn/arcvpn/internal/store"
	"github.com/arcvpn/arcvpn/internal/testfixture"
	"github.com/arcvpn/arcvpn/internal/usage"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestRealHysteriaOnlineIdleClientAndClosedClient(t *testing.T) {
	binary := os.Getenv("ARCVPN_TEST_HYSTERIA")
	if binary == "" {
		t.Skip("requires pinned real Hysteria in an isolated Linux network")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	certificate := filepath.Join(dir, "cert.pem")
	private := filepath.Join(dir, "private.pem")
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	public := &key.PublicKey
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "presence.test"}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour), DNSNames: []string{"presence.test"}, KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, BasicConstraintsValid: true, IsCA: true}
	der, err := x509.CreateCertificate(rand.Reader, template, template, public, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(certificate, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(private, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), 0600); err != nil {
		t.Fatal(err)
	}
	socket, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := socket.LocalAddr().(*net.UDPAddr).Port
	socket.Close()
	users := []store.User{{ID: 1, UUID: "11111111-1111-4111-8111-111111111111", Enabled: true}}
	credentials := []store.ProtocolCredential{{UserID: 1, Kind: "hysteria2", Secret: strings.Repeat("a", 43)}}
	profile := store.Protocol{Kind: "hysteria2", Address: "127.0.0.1", Port: port, SNI: "127.0.0.1", TLSCert: "/etc/arcvpn/tls/fullchain.pem", TLSKey: "/etc/arcvpn/tls/privkey.pem", Enabled: true}
	config, err := protocols.Render(profile, users, credentials)
	if err != nil {
		t.Fatal(err)
	}
	manager := Manager{Dir: dir}
	config, err = manager.withHysteriaStatistics(config)
	if err != nil {
		t.Fatal(err)
	}
	var isolated map[string]any
	if err = json.Unmarshal(config, &isolated); err != nil {
		t.Fatal(err)
	}
	// Test-only file paths; do not relax the production certificate allowlist.
	isolated["tls"] = map[string]string{"cert": certificate, "key": private, "sniGuard": "dns-san"}
	config, err = json.Marshal(isolated)
	if err != nil {
		t.Fatal(err)
	}
	serverPath := filepath.Join(dir, "hysteria.yaml")
	if err = os.WriteFile(serverPath, config, 0600); err != nil {
		t.Fatal(err)
	}
	server := testfixture.Start(t, binary, "--log-format", "json", "--disable-update-check", "server", "-c", serverPath)
	process := hysteriaProcess{PID: server.Cmd.Process.Pid, Epoch: "isolated-real-hysteria", StartedAt: time.Now().UnixNano()}
	query := func() ([]usage.PresenceObservation, error) {
		data, at, e := manager.hysteriaRead(context.Background(), users, credentials, process, "/online")
		if e != nil {
			return nil, e
		}
		return ParseHysteriaOnline(data, users, credentials, process.Epoch, at)
	}
	var lastError error
	check := func(want bool) bool {
		rows, e := query()
		lastError = e
		return e == nil && len(rows) == 1 && rows[0].Online != nil && *rows[0].Online == want
	}
	t.Cleanup(func() {
		if t.Failed() {
			t.Log("last guarded Hysteria query:", lastError)
		}
	})
	testfixture.Wait(t, "empty authenticated Hysteria online map", func() bool { return check(false) })
	proxy := testfixture.Port(t, "127.0.0.1")
	clientPath := filepath.Join(dir, "client.json")
	testfixture.WriteJSON(t, clientPath, map[string]any{"server": net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), "auth": "u1:" + credentials[0].Secret, "tls": map[string]any{"sni": "presence.test", "ca": certificate}, "socks5": map[string]any{"listen": net.JoinHostPort("127.0.0.1", strconv.Itoa(proxy))}})
	client := testfixture.Start(t, binary, "--log-format", "json", "--disable-update-check", "client", "-c", clientPath)
	testfixture.Wait(t, "Hysteria client SOCKS", func() bool {
		c, e := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(proxy)), 50*time.Millisecond)
		if e == nil {
			c.Close()
		}
		return e == nil
	})
	stream := testfixture.SOCKS(t, proxy, testfixture.Echo(t))
	testfixture.Transfer(t, stream)
	testfixture.Wait(t, "real Hysteria client online", func() bool { return check(true) })
	stream.Close()
	time.Sleep(250 * time.Millisecond)
	if !check(true) {
		t.Fatal("idle client was mistaken for a disconnected client")
	}
	client.Stop()
	testfixture.Wait(t, "QUIC client closure", func() bool { return check(false) })
	server.Stop()
	if _, err = query(); err == nil {
		t.Fatal("failed source became offline")
	}
	t.Log("real Hysteria: authenticated TLS/QUIC, proxy traffic, idle client remains online, stopped client offline, source failure unknown; local PID guard exercised, systemd wrapper not exercised")
}
