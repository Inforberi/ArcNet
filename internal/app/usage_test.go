package app

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/arcvpn/arcvpn/internal/db"
	"github.com/arcvpn/arcvpn/internal/store"
	"github.com/arcvpn/arcvpn/internal/xray"
)

func TestPackageApplyRetryAndUserSecrets(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	database, err := db.Open(filepath.Join(dir, "panel.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	st := &store.Store{DB: database}
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	binaryPath := filepath.Join(dir, "xray")
	script := `#!/bin/sh
base=$(dirname "$0")
if [ "$1" = x25519 ]; then
 echo 'PrivateKey: AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA'
 echo 'Password: BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB'
 exit 0
fi
if [ "$1" = api ];then cat "$base/stats.json";exit 0;fi
if [ "$1" = run ] && [ ! -f "$base/fail" ];then exit 0;fi
exit 1
`
	if err := os.WriteFile(binaryPath, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	a := &App{Store: st, Core: xray.Controller{Binary: binaryPath, Path: filepath.Join(dir, "xray.json"), SkipRestart: true}}
	if err := a.Setup(ctx, "198.51.100.25", "www.example.com", 443); err != nil {
		t.Fatal(err)
	}
	u, err := a.AddUser(ctx, "Alice")
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("unix", a.Core.StatisticsSocket())
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if err := os.Chmod(a.Core.StatisticsSocket(), 0600); err != nil {
		t.Fatal(err)
	}
	writeStats := func(text string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, "stats.json"), []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	writeStats(`{}`)
	if err := a.collectUsageLocked(ctx); err != nil {
		t.Fatal("fixture collection:", err)
	}
	if err := a.SaveUserPackage(ctx, u.ID, 100, 0, 0, false); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1100 * time.Millisecond)
	writeStats(`{"stat":[{"name":"user>>>user-1@local>>>traffic>>>uplink","value":"60"},{"name":"user>>>user-1@local>>>traffic>>>downlink","value":"40"}]}`)
	if err := a.collectUsageLocked(ctx); err != nil {
		t.Fatal(err)
	}
	if err := a.reconcilePackagesLocked(ctx, false); err != nil {
		t.Fatal(err)
	}
	if err := a.UserAllowed(ctx, u.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("exhausted user receives subscription", err)
	}
	config, _ := a.Core.Snapshot()
	if strings.Contains(string(config), u.UUID) {
		t.Fatal("exhausted user remains in generated config")
	}
	if err := os.WriteFile(filepath.Join(dir, "fail"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := a.SaveUserPackage(ctx, u.ID, 100, 0, 1, true); err != nil {
		t.Fatal(err)
	}
	state, _ := st.AccountingState(ctx)
	if state != "apply_failed" {
		t.Fatal("failed apply reported as success", state)
	}
	items, _ := st.Packages(ctx)
	if items[u.ID].Upload+items[u.ID].Download != 0 || items[u.ID].Period != 2 {
		t.Fatal("period not persisted")
	}
	if err := os.Remove(filepath.Join(dir, "fail")); err != nil {
		t.Fatal(err)
	}
	if err := a.reconcilePackagesLocked(ctx, false); err != nil {
		t.Fatal(err)
	}
	config, _ = a.Core.Snapshot()
	if !strings.Contains(string(config), u.UUID) {
		t.Fatal("retry did not restore allowed user")
	}
	actual, _ := st.ByID(ctx, u.ID)
	if actual.UUID != u.UUID || actual.Subscription != u.Subscription {
		t.Fatal("package changed secrets")
	}
	if _, err := a.AddProtocol(ctx, "hysteria2", "198.51.100.25", 0, "", ""); !errors.Is(err, store.ErrUsageUnsupported) {
		t.Fatal("unsupported engine bypassed package", err)
	}
	time.Sleep(1100 * time.Millisecond)
	writeStats(`{"stat":[{"name":"user>>>user-1@local>>>traffic>>>uplink","value":"70"},{"name":"user>>>user-1@local>>>traffic>>>downlink","value":"50"}]}`)
	if err := a.collectUsageLocked(ctx); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "fail"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if a.DeleteUser(ctx, u.ID) == nil {
		t.Fatal("expected failed engine apply")
	}
	actual, err = st.ByID(ctx, u.ID)
	if err != nil || actual.UUID != u.UUID || actual.Subscription != u.Subscription {
		t.Fatal("failed deletion lost user", err)
	}
	items, _ = st.Packages(ctx)
	if items[u.ID].Upload+items[u.ID].Download != 20 || items[u.ID].QuotaBytes != 100 {
		t.Fatal("failed deletion lost accounting", items[u.ID])
	}
}
