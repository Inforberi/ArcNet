package xray

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestOnlineListIsBoundedAndDoesNotReadAddresses(t *testing.T) {
	for _, raw := range []string{`{"users":["user-1@local"]}`, `{}`, `{"users":[]}`} {
		if _, err := ParseOnlineUsers([]byte(raw)); err != nil {
			t.Fatal(err)
		}
	}
	if names, err := ParseOnlineUsers([]byte(`{"users":["user>>>user-1@local>>>online"]}`)); err != nil || len(names) != 1 || names[0] != "user-1@local" {
		t.Fatal("native Xray online-map key", names, err)
	}
	for _, raw := range []string{`{"users":["a","a"]}`, `{"users":[""]}`, `{"ips":{"127.0.0.1":1}}`, `{"users":[]} {}`} {
		if _, err := ParseOnlineUsers([]byte(raw)); err == nil {
			t.Fatal("malformed presence accepted")
		}
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "xray")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nif [ \"$1\" = version ]; then echo 'Xray 25.1.1'; exit 0; fi\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := QueryOnlineUsers(context.Background(), bin, "/private/socket"); err == nil {
		t.Fatal("unverified legacy online semantics accepted")
	}
}
