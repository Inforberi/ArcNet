package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arcvpn/arcvpn/internal/auth"
	"github.com/arcvpn/arcvpn/internal/db"
	"github.com/arcvpn/arcvpn/internal/store"
)

func recoveryStore(t *testing.T) *store.Store {
	t.Helper()
	database, err := db.Open(filepath.Join(t.TempDir(), "panel.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	st := &store.Store{DB: database}
	if err = st.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err = st.CreateAdmin(context.Background(), "admin", "old-hash"); err != nil {
		t.Fatal(err)
	}
	return st
}
func TestResetAdministratorReadsStdinWithoutEchoAndRevokesSessions(t *testing.T) {
	st := recoveryStore(t)
	ctx := context.Background()
	if err := st.AddSession(ctx, "session", "csrf", 9999999999); err != nil {
		t.Fatal(err)
	}
	const password = " абвгде "
	var output bytes.Buffer
	if err := resetAdministrator(ctx, st, []string{"--password-stdin"}, strings.NewReader(password+"\n"), &output); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), password) {
		t.Fatal("supplied password echoed")
	}
	hash, _ := st.AdminHash(ctx, "admin")
	if !auth.VerifyPassword(password, hash) {
		t.Fatal("new password not usable")
	}
	if _, valid, _ := st.Session(ctx, "session"); valid {
		t.Fatal("old session survived")
	}
	for _, input := range []string{"12345678\r", "short", strings.Repeat("x", 257), strings.Repeat("x", 256) + "\r\nignored", "multi-line-password-2026\nsecond"} {
		if err := resetAdministrator(ctx, st, []string{"--password-stdin"}, strings.NewReader(input), &output); err == nil {
			t.Fatal("invalid password accepted")
		}
	}
	if err := resetAdministrator(ctx, st, []string{"--password", password}, strings.NewReader(""), &output); err == nil || strings.Contains(err.Error(), password) {
		t.Fatal("argv password accepted or leaked")
	}
}
func TestPanelSettingsProjectionRejectsSecretsAndInvalidProcesses(t *testing.T) {
	raw := []byte("/opt/arcvpn/bin/arcvpn-core\x00serve\x00--listen\x000.0.0.0:8443\x00--public-url=https://vpn.example.com:8443\x00--tls-cert\x00/private-cert-path\x00--tls-key\x00/private-key-path\x00--xray\x00private-secret\x00")
	settings, err := parsePanelArguments(raw)
	if err != nil || settings.Listen != "0.0.0.0:8443" || !settings.TLSConfigured {
		t.Fatalf("runtime: %#v %v", settings, err)
	}
	blob, _ := json.Marshal(settings)
	if strings.Contains(string(blob), "private-") {
		t.Fatal("process secrets exposed")
	}
	for _, bad := range []string{"/usr/bin/bash\x00serve\x00", "arcvpn-core\x00node-register\x00", "arcvpn-core\x00serve\x00--public-url=https://example.com/sub/secret\x00", "arcvpn-core\x00serve\x00--listen=bad\x00"} {
		if _, err := parsePanelArguments([]byte(bad)); err == nil {
			t.Fatal("invalid process accepted")
		}
	}
	for _, prefix := range []string{"--", "-"} {
		snapshot, err := parsePanelArguments([]byte("arcvpn-core\x00serve\x00" + prefix + "xray\x00--public-url=https://nested.example.com\x00" + prefix + "listen=127.0.0.1:8181\x00"))
		if err != nil || snapshot.PublicURL != "" || snapshot.Listen != "127.0.0.1:8181" {
			t.Fatal("option-looking value was interpreted as a setting")
		}
	}
	st := recoveryStore(t)
	var out bytes.Buffer
	if err := showLocalSettings(context.Background(), st, []string{"--json"}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"username":"admin"`) || strings.Contains(out.String(), "old-hash") {
		t.Fatal("unsafe settings output")
	}
}
func TestRootRecoveryFailsBeforeCreatingDatabase(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("Linux root fixture only")
	}
	original := os.Args
	t.Cleanup(func() { os.Args = original })
	dir := filepath.Join(t.TempDir(), "missing-controller")
	t.Setenv("ARCVPN_DATA_DIR", dir)
	os.Args = []string{"arcvpn-core", "admin-reset"}
	if err := run(); err == nil {
		t.Fatal("core recovery ran as root")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("unauthorized recovery created files")
	}
}
