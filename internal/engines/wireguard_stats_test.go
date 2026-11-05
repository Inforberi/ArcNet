package engines

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/arcvpn/arcvpn/internal/devices"
	"github.com/arcvpn/arcvpn/internal/protocols"
	"github.com/arcvpn/arcvpn/internal/store"
)

func TestWireGuardPublicReportValidationAndDirections(t *testing.T) {
	p, u, c := newWireGuardProfile(t)
	config, err := protocols.Render(p, []store.User{u}, []store.ProtocolCredential{c})
	if err != nil {
		t.Fatal(err)
	}
	receipt := WireGuardReceipt{Epoch: "boot/10/random", StartedAt: 1, Index: 10, ConfigSHA: wireGuardDigest(config)}
	raw := []byte(c.PublicKey + "\t123\t456\n")
	report, err := checkedWireGuardReport(receipt, config, []byte(p.PublicKey), raw)
	if err != nil || report.Peers[0].Upload != 123 || report.Peers[0].Download != 456 {
		t.Fatal("rx/tx direction", err)
	}
	blob, _ := json.Marshal(report)
	if strings.Contains(string(blob), p.PrivateKey) || strings.Contains(string(blob), c.PrivateKey) {
		t.Fatal("private key in report")
	}
	for _, bad := range []string{c.PublicKey + " 1 -1", c.PublicKey + " 1", string(raw) + string(raw), "invalid 1 2"} {
		if _, err := ParseWireGuardTransfers([]byte(bad)); err == nil {
			t.Fatal("malformed/duplicate counters accepted")
		}
	}
	bad := receipt
	bad.ConfigSHA = "wrong"
	if _, err := checkedWireGuardReport(bad, config, []byte(p.PublicKey), raw); err == nil {
		t.Fatal("wrong revision")
	}
	if _, err := checkedWireGuardReport(receipt, config, []byte(c.PublicKey), raw); err == nil {
		t.Fatal("wrong server identity")
	}
	if _, err := checkedWireGuardReport(receipt, config, []byte(p.PublicKey), nil); err == nil {
		t.Fatal("missing peer treated as zero")
	}
	u.UUID = "11111111-1111-4111-8111-111111111111"
	counters, err := wireGuardCounters(report, []store.User{u}, []store.ProtocolCredential{c}, time.Now().Unix())
	if err != nil || counters[0].Upload != 123 || counters[0].Download != 456 {
		t.Fatal(counters, err)
	}
}
func TestWireGuardDeviceRenderingAndClient(t *testing.T) {
	p, u, c := newWireGuardProfile(t)
	priv, pub, _ := devices.NewWireGuardKeys()
	d := devices.Device{WGPrivate: priv, WGPublic: pub, WGAddress: "10.76.255.254"}
	u.DeviceAccess = &devices.Record{Devices: []devices.Device{d}}
	config, err := protocols.Render(p, []store.User{u}, []store.ProtocolCredential{c})
	if err != nil || strings.Contains(string(config), c.PublicKey) || !strings.Contains(string(config), pub) {
		t.Fatal("common peer bypass", err)
	}
	if err := ValidateQuickServer("wireguard", config); err != nil {
		t.Fatal(err)
	}
	client, err := protocols.ClientTunnel(p, store.ProtocolCredential{UserID: u.ID, PrivateKey: priv, PublicKey: pub, Address: d.WGAddress})
	if err != nil || !strings.Contains(client, "Address = "+d.WGAddress+"/32") {
		t.Fatal("client configuration", err)
	}
	u.DeviceAccess.Devices[0].RevokedAt = 100
	config, err = protocols.Render(p, []store.User{u}, []store.ProtocolCredential{c})
	if err != nil || strings.Contains(string(config), pub) {
		t.Fatal("revoked peer rendered", err)
	}
}
func TestWireGuardHelperReportTransport(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root-owned helper fixture, run Linux container")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(dir, "helper.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	p, u, c := newWireGuardProfile(t)
	u.UUID = "11111111-1111-4111-8111-111111111111"
	config, _ := protocols.Render(p, []store.User{u}, []store.ProtocolCredential{c})
	if err := os.WriteFile(filepath.Join(dir, "arcwg.conf"), config, 0600); err != nil {
		t.Fatal(err)
	}
	report, _ := checkedWireGuardReport(WireGuardReceipt{Epoch: "test", StartedAt: 1, Index: 1, ConfigSHA: wireGuardDigest(config)}, config, []byte(p.PublicKey), []byte(c.PublicKey+"\t10\t20"))
	done := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			done <- err
			return
		}
		defer conn.Close()
		request := make([]byte, 32)
		n, err := conn.Read(request)
		if err != nil {
			done <- err
			return
		}
		if string(request[:n]) != "stats wireguard\n" {
			done <- fmt.Errorf("unexpected request")
			return
		}
		done <- json.NewEncoder(conn).Encode(report)
	}()
	m := Manager{Dir: dir, HelperSocket: socket}
	counters, _, err := m.WireGuardStatistics(context.Background(), []store.User{u}, []store.ProtocolCredential{c})
	if err != nil || counters[0].Upload != 10 {
		t.Fatal(counters, err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
