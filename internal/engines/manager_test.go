package engines

import (
	"bufio"
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/arcvpn/arcvpn/internal/protocols"
	"github.com/arcvpn/arcvpn/internal/store"
)

func newWireGuardProfile(t *testing.T) (store.Protocol, store.User, store.ProtocolCredential) {
	t.Helper()
	p, err := protocols.New(context.Background(), protocols.WireGuard, "198.51.100.2", 51820, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	u := store.User{ID: 1, Enabled: true}
	c, err := protocols.NewCredential(protocols.WireGuard, u)
	if err != nil {
		t.Fatal(err)
	}
	return p, u, c
}

func TestApplyPreservesExistingEngineWhenNewConfigCannotBeValidated(t *testing.T) {
	dir := t.TempDir()
	old := []byte("old hysteria credentials, must not be deleted\n")
	if err := os.WriteFile(filepath.Join(dir, "hysteria.yaml"), old, 0600); err != nil {
		t.Fatal(err)
	}
	p, u, c := newWireGuardProfile(t)
	mgr := Manager{Dir: dir, WGQuickBinary: filepath.Join(dir, "not-installed")}
	if err := mgr.Apply(context.Background(), []store.Protocol{p}, []store.User{u}, []store.ProtocolCredential{c}); err == nil {
		t.Fatal("expected validation failure without WireGuard runtime")
	}
	data, err := os.ReadFile(filepath.Join(dir, "hysteria.yaml"))
	if err != nil || string(data) != string(old) {
		t.Fatalf("old credentials were modified before validation: %v %q", err, data)
	}
	if _, err := os.Stat(filepath.Join(dir, "arcwg.conf")); !os.IsNotExist(err) {
		t.Fatalf("candidate config should not have been installed: %v", err)
	}
}

func TestApplyRestoresRemovedEngineAndItsServiceStateAfterFailure(t *testing.T) {
	dir := t.TempDir()
	old := []byte("old hysteria credentials must survive a failed restore\n")
	if err := os.WriteFile(filepath.Join(dir, "hysteria.yaml"), old, 0600); err != nil {
		t.Fatal(err)
	}
	quick := filepath.Join(dir, "wg-quick")
	if err := os.WriteFile(quick, []byte("#!/bin/sh\n[ \"$1\" = strip ]\n"), 0700); err != nil {
		t.Fatal(err)
	}
	control := filepath.Join(dir, "control.sock")
	listener, err := net.Listen("unix", control)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	var mu sync.Mutex
	var commands []string
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			conn, e := listener.Accept()
			if e != nil {
				return
			}
			func() {
				defer conn.Close()
				line, e := bufio.NewReader(conn).ReadString('\n')
				if e != nil {
					return
				}
				mu.Lock()
				commands = append(commands, strings.TrimSpace(line))
				mu.Unlock()
				reply := "ok\n"
				switch line {
				case "status hysteria\n":
					reply = "active\n"
				case "status wireguard\n", "status amneziawg\n":
					reply = "inactive\n"
				case "restart wireguard\n":
					reply = "error\n" // Simulate kernel or systemd activation failure.
				}
				_, _ = conn.Write([]byte(reply))
			}()
		}
	}()
	defer func() { listener.Close(); wg.Wait() }()
	p, u, c := newWireGuardProfile(t)
	mgr := Manager{Dir: dir, WGQuickBinary: quick, HelperSocket: control}
	if err := mgr.Apply(context.Background(), []store.Protocol{p}, []store.User{u}, []store.ProtocolCredential{c}); err == nil {
		t.Fatal("expected simulated service activation failure")
	}
	got, err := os.ReadFile(filepath.Join(dir, "hysteria.yaml"))
	if err != nil || string(got) != string(old) {
		t.Fatalf("deleted service file not restored: %v %q", err, got)
	}
	if _, err := os.Stat(filepath.Join(dir, "arcwg.conf")); !os.IsNotExist(err) {
		t.Fatalf("failed candidate remains installed: %v", err)
	}
	mu.Lock()
	cmds := append([]string(nil), commands...)
	mu.Unlock()
	joined := strings.Join(cmds, ";")
	for _, required := range []string{"status hysteria", "stop hysteria", "restart wireguard", "restart hysteria", "stop wireguard"} {
		if !strings.Contains(joined, required) {
			t.Errorf("operation %q missing from helper history: %v", required, cmds)
		}
	}
}
