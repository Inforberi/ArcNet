package engines

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/arcvpn/arcvpn/internal/devices"
	"github.com/arcvpn/arcvpn/internal/protocols"
	"github.com/arcvpn/arcvpn/internal/store"
)

func TestHysteriaProcessAndCounterValidation(t *testing.T) {
	p, err := parseHysteriaProcess([]byte("MainPID=123\nInvocationID=" + strings.Repeat("a", 32) + "\nExecMainStartTimestamp=Thu 2026-11-05 13:14:15.123456 UTC\n"))
	if err != nil || p.PID != 123 || p.StartedAt != time.Date(2026, 11, 5, 13, 14, 15, 123456000, time.UTC).UnixNano() {
		t.Fatal(p, err)
	}
	for _, blob := range []string{"", "MainPID=0\n", "MainPID=123\nInvocationID=invalid\n"} {
		if _, err := parseHysteriaProcess([]byte(blob)); err == nil {
			t.Fatal("invalid process accepted")
		}
	}
	d := devices.Device{ID: "22222222-2222-4222-8222-222222222222", UUID: "33333333-3333-4333-8333-333333333333", RevokedAt: 100}
	users := []store.User{{ID: 1, UUID: "11111111-1111-4111-8111-111111111111", DeviceAccess: &devices.Record{Devices: []devices.Device{d}}}}
	credentials := []store.ProtocolCredential{{UserID: 1, Kind: protocols.Hysteria2, Secret: strings.Repeat("x", 43)}}
	stats := `{"u1":{"tx":10,"rx":20},"` + protocols.HysteriaDeviceName(d.ID) + `":{"tx":30,"rx":40}}`
	counters, err := ParseHysteriaStatistics([]byte(stats), users, credentials, p, time.Now().Unix())
	if err != nil || len(counters) != 1 || counters[0].Upload != 40 || counters[0].Download != 60 {
		t.Fatal(counters, err)
	}
	for _, blob := range []string{`null`, `[]`, `{} {}`, `{"u1":{"tx":-1}}`, `{"u1":{"rx":1.5}}`, `{"u1":{"tx":9000000000000001}}`} {
		if _, err := ParseHysteriaStatistics([]byte(blob), users, credentials, p, time.Now().Unix()); err == nil {
			t.Fatal("invalid counters accepted", blob)
		}
	}
}

func TestHysteriaListenerMustBeOwnedByExpectedProcess(t *testing.T) {
	listener, err := net.Listen("tcp4", hysteriaStatsAddress)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if err := verifyHysteriaListener(hysteriaProcess{PID: os.Getpid()}); err != nil {
		t.Fatal("own listener", err)
	}
	if err := verifyHysteriaListener(hysteriaProcess{PID: os.Getppid()}); err == nil {
		t.Fatal("wrong owner accepted")
	}
}

func TestHysteriaPrivateConfigAndNonResettingHTTP(t *testing.T) {
	listener, err := net.Listen("tcp4", hysteriaStatsAddress)
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/traffic" || r.URL.Query().Get("clear") != "false" || r.Header.Get("Authorization") == "" {
			t.Error("unsafe statistics request")
			w.WriteHeader(403)
			return
		}
		_, _ = fmt.Fprint(w, `{"u1":{"tx":10,"rx":20}}`)
	})}
	go server.Serve(listener)
	defer server.Close()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	m := Manager{Dir: dir}
	p := store.Protocol{Kind: protocols.Hysteria2, Address: "127.0.0.1", SNI: "127.0.0.1", Port: 8444, TLSCert: "/etc/arcvpn/tls/fullchain.pem", TLSKey: "/etc/arcvpn/tls/privkey.pem"}
	users := []store.User{{ID: 1, UUID: "11111111-1111-4111-8111-111111111111", Enabled: true}}
	creds := []store.ProtocolCredential{{UserID: 1, Kind: protocols.Hysteria2, Secret: strings.Repeat("a", 43)}}
	data, err := protocols.Render(p, users, creds)
	if err != nil {
		t.Fatal(err)
	}
	data, err = m.withHysteriaStatistics(data)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "hysteria.yaml")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	process := hysteriaProcess{PID: os.Getpid(), Epoch: strings.Repeat("a", 32), StartedAt: 1}
	for i := 0; i < 2; i++ {
		counters, _, err := m.hysteriaStatistics(context.Background(), users, creds, process)
		if err != nil || counters[0].Upload != 10 || counters[0].Download != 20 {
			t.Fatal(counters, err)
		}
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := m.hysteriaStatistics(context.Background(), users, creds, process); err == nil {
		t.Fatal("public config accepted")
	}
}
