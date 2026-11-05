package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arcvpn/arcvpn/internal/backup"
	"github.com/arcvpn/arcvpn/internal/db"
	"github.com/arcvpn/arcvpn/internal/devices"
	"github.com/arcvpn/arcvpn/internal/store"
	"github.com/arcvpn/arcvpn/internal/xray"
)

func TestDeviceLifecycleRetryAndParentAccess(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	database, err := db.Open(filepath.Join(dir, "panel.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	st := &store.Store{DB: database}
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(dir, "xray")
	script := `#!/bin/sh
if [ "$1" = x25519 ];then
echo 'PrivateKey: AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA'
echo 'Password: BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB'
exit 0
fi
if [ "$1" = run ] && [ ! -f "$(dirname "$0")/fail" ];then exit 0;fi
exit 1
`
	if err := os.WriteFile(binary, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	a := &App{Store: st, Core: xray.Controller{Binary: binary, Path: filepath.Join(dir, "xray.json"), SkipRestart: true}}
	if err := a.Setup(ctx, "198.51.100.25", "www.example.com", 443); err != nil {
		t.Fatal(err)
	}
	u, err := a.AddUser(ctx, "Alice")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.ChangeUserDevices(ctx, u.ID, 0, "Phone", "", nil); err != nil {
		t.Fatal(err)
	}
	record, _ := st.UserDevices(ctx, u.ID)
	d := record.Devices[0]
	config, _ := a.Core.Snapshot()
	if strings.Contains(string(config), u.UUID) || !strings.Contains(string(config), d.UUID) {
		t.Fatal("common credential bypass")
	}
	if _, err := a.SubscriptionLinks(ctx, u); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("common subscription remains", err)
	}
	if _, err := a.DeviceLinks(ctx, u.ID, d.ID); err != nil {
		t.Fatal(err)
	}
	// A failed import may add a credential to desired DB state, but its
	// compensation must remove it, while preserving all original credentials.
	prior, err := a.ExportState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	blob, _ := json.Marshal(prior)
	var candidate backup.Snapshot
	if err := json.Unmarshal(blob, &candidate); err != nil {
		t.Fatal(err)
	}
	imported := devices.Device{ID: "66666666-6666-4666-8666-666666666666", UUID: "77777777-7777-4777-8777-777777777777", Token: strings.Repeat("z", 43), Name: "Imported", CreatedAt: 100}
	(*candidate.Devices)[0].Devices = append((*candidate.Devices)[0].Devices, imported)
	if err := os.WriteFile(filepath.Join(dir, "fail"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if a.RestoreState(ctx, candidate) == nil {
		t.Fatal("expected failed import")
	}
	got, err := st.UserDevices(ctx, u.ID)
	if err != nil || len(got.Devices) != 1 || got.Devices[0].UUID != d.UUID {
		t.Fatal("failed import leaked device", got, err)
	}
	if _, _, err := st.DeviceByToken(ctx, imported.Token); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("failed import left token", err)
	}
	if err := os.Remove(filepath.Join(dir, "fail")); err != nil {
		t.Fatal(err)
	}
	one := 1
	if err := a.ChangeUserDevices(ctx, u.ID, 1, "", "", &one); err != nil {
		t.Fatal(err)
	}
	if err := a.ChangeUserDevices(ctx, u.ID, 2, "Laptop", "", nil); !errors.Is(err, devices.ErrLimit) {
		t.Fatal("limit bypass", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "fail"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := a.ChangeUserDevices(ctx, u.ID, 2, "", d.ID, nil); err != nil {
		t.Fatal(err)
	}
	state, _ := st.AccountingState(ctx)
	if state != "apply_failed" {
		t.Fatal("failed apply hidden", state)
	}
	if _, err := a.DeviceLinks(ctx, u.ID, d.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("revoked subscription survives")
	}
	config, _ = a.Core.Snapshot()
	if !strings.Contains(string(config), d.UUID) {
		t.Fatal("test did not retain previous config")
	}
	if err := os.Remove(filepath.Join(dir, "fail")); err != nil {
		t.Fatal(err)
	}
	if err := a.reconcilePackagesLocked(ctx, false); err != nil {
		t.Fatal(err)
	}
	config, _ = a.Core.Snapshot()
	if strings.Contains(string(config), d.UUID) {
		t.Fatal("retry did not revoke credential")
	}
	if err := a.ChangeUserDevices(ctx, u.ID, 3, "Laptop", "", nil); err != nil {
		t.Fatal("revoked slot not reusable", err)
	}
	record, _ = st.UserDevices(ctx, u.ID)
	next := record.Devices[1]
	if err := a.ToggleUser(ctx, u.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DeviceLinks(ctx, u.ID, next.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("disabled parent bypass")
	}
	if err := a.ToggleUser(ctx, u.ID, true); err != nil {
		t.Fatal(err)
	}
	currentPackage, err := st.Package(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SavePackage(ctx, u.ID, 0, 1, currentPackage.Version, false); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DeviceLinks(ctx, u.ID, next.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("expired parent bypass")
	}
}
