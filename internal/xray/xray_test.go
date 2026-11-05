package xray

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arcvpn/arcvpn/internal/store"
)

const testPrivate = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
const testPublic = "BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB"

func TestGenerateValidateAndLink(t *testing.T) {
	cfg := store.Reality{Address: "198.51.100.15", Port: 443, Target: "www.example.com", SNI: "www.example.com", Private: testPrivate, Public: testPublic, ShortID: "0123456789abcdef", Enabled: true}
	users := []store.User{{Name: "Phone", UUID: "c4d00000-0000-4000-8000-000000000000", Enabled: true}, {Name: "Disabled", UUID: "c4d00000-0000-4000-8000-000000000001", Enabled: false}}
	b, err := Generate(cfg, users)
	if err != nil {
		t.Fatal(err)
	}
	var output map[string]any
	if err := json.Unmarshal(b, &output); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "000000000001") {
		t.Fatal("disabled client included")
	}
	link := Link(cfg, users[0])
	if !strings.Contains(link, "security=reality") || !strings.Contains(link, "pbk=") {
		t.Fatal(link)
	}
	cfg.ShortID = "invalid"
	if _, err := Generate(cfg, users); err == nil {
		t.Fatal("invalid shortId accepted")
	}
}
func TestApplyPreflightAndAtomicWrite(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "xray")
	script := "#!/bin/sh\nif [ \"$1\" = 'x25519' ]; then echo 'PrivateKey: " + testPrivate + "';echo 'Password: " + testPublic + "';exit 0;fi\nif [ \"$1\" = 'run' ] && [ \"$2\" = '-test' ]; then grep -q 'vless' \"$4\";exit $?;fi\nexit 9\n"
	if err := os.WriteFile(binary, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	cfg := store.Reality{Address: "198.51.100.15", Port: 443, Target: "www.example.com", SNI: "www.example.com", Private: testPrivate, Public: testPublic, ShortID: "0123456789abcdef", Enabled: true}
	control := Controller{Binary: binary, Path: filepath.Join(dir, "config.json"), SkipRestart: true}
	if err := control.Apply(context.Background(), cfg, nil); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(control.Path)
	if err != nil || !strings.Contains(string(b), "vless") {
		t.Fatalf("config not written: %v", err)
	}
	if err := control.Apply(context.Background(), cfg, nil); err != nil {
		t.Fatal("reapply failed", err)
	}
	priv, pub, sid, err := GenerateKeys(context.Background(), binary)
	if err != nil || priv != testPrivate || pub != testPublic || len(sid) != 16 {
		t.Fatalf("key parsing: %v", err)
	}
}

func TestLinkIPv6(t *testing.T) {
	r := store.Reality{Address: "2001:db8::1", Port: 443, SNI: "www.example.com", Public: "BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB", ShortID: "aabbccdd00112233"}
	u := store.User{Name: "IPv6 Phone", UUID: "12345678-1234-4234-8234-123456789abc"}
	link := Link(r, u)
	if !strings.Contains(link, "@[2001:db8::1]:443?") {
		t.Fatalf("invalid IPv6 subscription URI: %s", link)
	}
}
