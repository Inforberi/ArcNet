package engines

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arcvpn/arcvpn/internal/store"
)

func TestOperationErrorHidesCause(t *testing.T) {
	cause := errors.New("private-key-secret")
	err := &OperationError{Kind: "wireguard", Stage: "validation", Err: cause}
	if strings.Contains(err.Error(), cause.Error()) || !errors.Is(err, cause) {
		t.Fatal("unsafe error or lost cause")
	}
}

func TestApplyValidationFailureIsTypedAndPreservesFiles(t *testing.T) {
	dir := t.TempDir()
	old := []byte("previous configuration")
	if err := os.WriteFile(filepath.Join(dir, "arcwg.conf"), old, 0600); err != nil {
		t.Fatal(err)
	}
	quick := filepath.Join(dir, "wg-quick")
	if err := os.WriteFile(quick, []byte("#!/bin/sh\necho private-key-secret >&2\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	p, u, c := newWireGuardProfile(t)
	mgr := Manager{Dir: dir, WGQuickBinary: quick}
	err := mgr.Apply(context.Background(), []store.Protocol{p}, []store.User{u}, []store.ProtocolCredential{c})
	var operation *OperationError
	if !errors.As(err, &operation) || operation.Kind != "wireguard" || operation.Stage != "validation" || strings.Contains(err.Error(), "private-key-secret") {
		t.Fatalf("unexpected error: %v", err)
	}
	got, readErr := os.ReadFile(filepath.Join(dir, "arcwg.conf"))
	if readErr != nil || string(got) != string(old) {
		t.Fatal("previous configuration changed")
	}
}
