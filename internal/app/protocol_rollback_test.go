package app

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arcvpn/arcvpn/internal/db"
	"github.com/arcvpn/arcvpn/internal/engines"
	"github.com/arcvpn/arcvpn/internal/protocols"
	"github.com/arcvpn/arcvpn/internal/store"
	"github.com/arcvpn/arcvpn/internal/xray"
)

// A configured XHTTP listener changes the Xray JSON. An auxiliary service
// failure after that step must restore the previous Xray file, not merely the DB.
func TestAuxiliaryEngineFailureRestoresXrayAndDatabase(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	database, err := db.Open(filepath.Join(dir, "arcvpn.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	st := &store.Store{DB: database}
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(dir, "fake-xray")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nif [ \"$1\" = x25519 ]; then echo 'PrivateKey: AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA'; echo 'Password: BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB'; exit 0; fi\nif [ \"$1\" = run ]; then exit 0; fi\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	manager := &engines.Manager{Dir: filepath.Join(dir, "aux"), SkipRestart: true}
	xrayPath := filepath.Join(dir, "xray.json")
	a := &App{Store: st, Core: xray.Controller{Binary: binary, Path: xrayPath, SkipRestart: true}, Engines: manager}
	if err := a.Setup(ctx, "198.51.100.19", "www.example.com", 443); err != nil {
		t.Fatal(err)
	}
	if _, err := a.AddUser(ctx, "Alice"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.AddProtocol(ctx, protocols.Hysteria2, "198.51.100.19", 0, "", ""); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(xrayPath)
	if err != nil {
		t.Fatal(err)
	}
	// The Hysteria binary is deliberately absent; Xray accepts the XHTTP
	// candidate before the auxiliary engine validation fails.
	manager.SkipRestart = false
	manager.HysteriaBinary = filepath.Join(dir, "missing-hysteria")
	_, err = a.AddProtocol(ctx, protocols.XHTTP, "198.51.100.19", 2087, "", "")
	if err == nil {
		t.Fatal("expected auxiliary engine failure")
	}
	if !strings.Contains(err.Error(), "auxiliary engine") {
		t.Fatalf("missing auxiliary engine failure context: %v", err)
	}
	after, err := os.ReadFile(xrayPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("Xray was left with the failed XHTTP inbound")
	}
	if _, err := st.GetProtocol(ctx, protocols.XHTTP); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("failed XHTTP listener persists in DB: %v", err)
	}
}

func TestFailedInitialSetupRemovesUnappliedXrayConfig(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	database, err := db.Open(filepath.Join(dir, "arcvpn.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	st := &store.Store{DB: database}
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(dir, "fake-xray")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nif [ \"$1\" = x25519 ]; then echo 'PrivateKey: AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA'; echo 'Password: BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB'; exit 0; fi\nif [ \"$1\" = run ]; then exit 0; fi\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	// No control socket: auxiliary status cannot be read, setup must abort.
	path := filepath.Join(dir, "xray.json")
	a := &App{Store: st, Core: xray.Controller{Binary: binary, Path: path, SkipRestart: true}, Engines: &engines.Manager{Dir: filepath.Join(dir, "aux")}}
	if err := a.Setup(ctx, "198.51.100.19", "www.example.com", 443); err == nil {
		t.Fatal("expected setup failure")
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Xray config left behind after failed initial setup: %v", err)
	}
	if _, err := st.ReadReality(ctx); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("database contains failed reality config: %v", err)
	}
}
