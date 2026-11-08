package app

import (
	"context"
	"github.com/arcvpn/arcvpn/internal/store"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestUserEditorPendingRuntimeRetries(t *testing.T) {
	a := firstProtocolApp(t)
	ctx := context.Background()
	if err := a.Setup(ctx, "198.51.100.19", "www.example.com", 443); err != nil {
		t.Fatal(err)
	}
	u, err := a.AddUser(ctx, "Alice")
	if err != nil {
		t.Fatal(err)
	}
	if err = a.reconcilePackagesLocked(ctx, true); err != nil {
		t.Fatal(err)
	}
	_, p, err := a.Store.UserEditor(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	fail := filepath.Join(filepath.Dir(a.Core.Binary), "fail")
	if err = os.WriteFile(fail, []byte("1"), 0600); err != nil {
		t.Fatal(err)
	}
	in := store.UserEdit{Name: "Saved together", Preferences: u.Preferences, ExpectedEnabled: true, Enabled: true, Package: &store.UserPackageEdit{ExpiresAt: time.Now().Add(time.Hour).Unix(), ExpectedVersion: p.Version}}
	if err = a.SaveUserEditor(ctx, u.ID, in); err != nil {
		t.Fatal(err)
	}
	got, p, err := a.Store.UserEditor(ctx, u.ID)
	if err != nil || got.Name != in.Name || p.ExpiresAt != in.Package.ExpiresAt || got.UUID != u.UUID {
		t.Fatal("pending save lost settings", err)
	}
	if state, _ := a.Store.AccountingState(ctx); state != "apply_failed" {
		t.Fatal(state)
	}
	if err = os.Remove(fail); err != nil {
		t.Fatal(err)
	}
	// A package-only change can leave the digest unchanged; pending state must still retry.
	if err = a.reconcilePackagesLocked(ctx, false); err != nil {
		t.Fatal(err)
	}
	if state, _ := a.Store.AccountingState(ctx); state != "unverified" {
		t.Fatal("retry skipped", state)
	}
}
