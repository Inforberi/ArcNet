package updater

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/arcvpn/arcvpn/internal/lifecycle"
)

func TestUpdaterRejectsLifecycleLockAndStoppedRuntimeBeforeStaging(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root coordination fixture")
	}
	dir := t.TempDir()
	public, _, _ := ed25519.GenerateKey(rand.Reader)
	publicPath := filepath.Join(dir, "public.hex")
	if err := os.WriteFile(publicPath, []byte(hex.EncodeToString(public)), 0644); err != nil {
		t.Fatal(err)
	}
	guard := lifecycle.Guard{Dir: filepath.Join(dir, "operations")}
	installer := Installer{PublicKey: publicPath, LockFile: filepath.Join(dir, "update.lock"), Coordination: &guard, Staged: filepath.Join(dir, "must-not-be-read")}
	unlock, err := guard.Lock()
	if err != nil {
		t.Fatal(err)
	}
	if err := installer.Apply(context.Background()); !errors.Is(err, lifecycle.ErrBusy) {
		t.Fatal("update overlapped lifecycle", err)
	}
	if err := guard.Pause(); err != nil {
		t.Fatal(err)
	}
	unlock()
	if err := installer.Apply(context.Background()); !errors.Is(err, lifecycle.ErrPaused) {
		t.Fatal("update woke stopped installation", err)
	}
}
