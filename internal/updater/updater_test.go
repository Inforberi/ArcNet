package updater

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arcvpn/arcvpn/internal/lifecycle"
)

func TestSignVerifyAndRejectTampering(t *testing.T) {
	pub, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	binary := []byte(strings.Repeat("real program bytes", 500))
	packageBytes, err := MakePackage(binary, "0.5.1", private)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := Verify(packageBytes, pub)
	if err != nil || payload.Manifest.Version != "0.5.1" || string(payload.Binary) != string(binary) {
		t.Fatal(err)
	}
	other, _, _ := ed25519.GenerateKey(rand.Reader)
	if _, e := Verify(packageBytes, other); e == nil {
		t.Fatal("wrong signing key accepted")
	}
	idx := bytes.LastIndex(packageBytes, []byte("manifest.json"))
	if idx < 0 {
		t.Fatal("signed manifest not found")
	}
	packageBytes[idx] ^= 0x01
	if _, e := Verify(packageBytes, pub); e == nil {
		t.Fatal("tampered archive accepted")
	}
}
func TestKeyFiles(t *testing.T) {
	dir := t.TempDir()
	if err := GenerateKeyPair(dir); err != nil {
		t.Fatal(err)
	}
	if err := GenerateKeyPair(dir); err == nil {
		t.Fatal("overwrote a signing key")
	}
	st, err := os.Stat(filepath.Join(dir, "update-private.hex"))
	if err != nil || st.Mode().Perm() != 0600 {
		t.Fatal("private key permissions", err)
	}
	pub, err := ReadPublicKey(filepath.Join(dir, "update-public.hex"))
	if err != nil {
		t.Fatal(err)
	}
	priv, err := ReadPrivateKey(filepath.Join(dir, "update-private.hex"))
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(priv.Public().(ed25519.PublicKey)) != hex.EncodeToString(pub) {
		t.Fatal("public mismatch")
	}
}

type fakeRunner struct {
	failRestart bool
	calls       []string
}

func (r *fakeRunner) Run(_ context.Context, name string, args ...string) error {
	r.calls = append(r.calls, name+" "+strings.Join(args, " "))
	if strings.HasSuffix(name, "systemctl") && len(args) > 0 && args[0] == "restart" && r.failRestart {
		r.failRestart = false
		return errors.New("service cannot start")
	}
	return nil
}
func TestInstallerRollback(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root lifecycle coordination fixture runs in disposable Linux")
	}
	pub, private, _ := ed25519.GenerateKey(rand.Reader)
	dir := t.TempDir()
	old := filepath.Join(dir, "arcvpn-core")
	if err := os.WriteFile(old, []byte("old program"), 0755); err != nil {
		t.Fatal(err)
	}
	signed, err := MakePackage([]byte(strings.Repeat("new program", 500)), "0.5.1", private)
	if err != nil {
		t.Fatal(err)
	}
	staged := filepath.Join(dir, "staged.zip")
	if err := os.WriteFile(staged, signed, 0600); err != nil {
		t.Fatal(err)
	}
	public := filepath.Join(dir, "public.hex")
	if err := os.WriteFile(public, []byte(hex.EncodeToString(pub)), 0644); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{failRestart: true}
	install := Installer{Binary: old, Staged: staged, PublicKey: public, LockFile: filepath.Join(dir, "lock"), Runner: runner, Coordination: &lifecycle.Guard{Dir: filepath.Join(dir, "lifecycle")}}
	if _, err := install.Verify(); err != nil {
		t.Fatal(err)
	}
	err = install.Apply(context.Background())
	if err == nil || !strings.Contains(err.Error(), "restored") {
		t.Fatal("rollback did not report failure", err)
	}
	actual, err := os.ReadFile(old)
	if err != nil || string(actual) != "old program" {
		t.Fatal("previous version not restored", err)
	}
}

func TestStagedArchiveRejectsSymlinksAndOversize(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "signed.zip")
	if err := os.WriteFile(real, []byte("small"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "stage-link.zip")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	if _, err := readStagedArchive(link); !errors.Is(err, ErrInvalid) {
		t.Fatal("symlink accepted", err)
	}
	large := filepath.Join(dir, "large.zip")
	file, err := os.OpenFile(large, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if err = file.Truncate(MaxArchive + 1); err != nil {
		t.Fatal(err)
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := readStagedArchive(large); !errors.Is(err, ErrInvalid) {
		t.Fatal("oversized sparse file accepted", err)
	}
}

func TestSignedMultiComponentPackage(t *testing.T) {
	pub, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	binary := []byte(strings.Repeat("arcvpn-runtime", 150))
	extras := map[string][]byte{
		"xray":         []byte("signed xray component"),
		"hysteria":     []byte("signed hysteria component"),
		"awg":          []byte("signed awg component"),
		"awg-quick":    []byte("signed awg quick component"),
		"amneziawg-go": []byte("signed userspace daemon"),
	}
	archive, err := MakePackageWithComponents(binary, "0.5.1", private, extras)
	if err != nil {
		t.Fatal(err)
	}
	out, err := Verify(archive, pub)
	if err != nil || len(out.Components) != len(extras) {
		t.Fatalf("multi-component update not verified: %v", err)
	}
	for name, want := range extras {
		if !bytes.Equal(want, out.Components[name]) {
			t.Fatalf("component %s lost its contents", name)
		}
	}
	// Signed files must not be modified after manifest signing.
	wrong := ed25519.PublicKey(make([]byte, ed25519.PublicKeySize))
	if _, err := Verify(archive, wrong); !errors.Is(err, ErrInvalid) {
		t.Fatalf("untrusted component archive accepted: %v", err)
	}
}

func TestKeyReaderRejectsSymlinksOversizedAndExposedPrivateKeys(t *testing.T) {
	root := t.TempDir()
	if err := GenerateKeyPair(root); err != nil {
		t.Fatal(err)
	}
	pub := filepath.Join(root, "update-public.hex")
	priv := filepath.Join(root, "update-private.hex")
	link := filepath.Join(root, "public-link.hex")
	if err := os.Symlink(pub, link); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadPublicKey(link); err == nil {
		t.Fatal("public trust anchor symlink accepted")
	}
	if err := os.Chmod(priv, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadPrivateKey(priv); err == nil {
		t.Fatal("exposed private signing key accepted")
	}
	if err := os.Chmod(priv, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadPrivateKey(priv); err != nil {
		t.Fatal("valid private signing key refused", err)
	}
	if err := os.WriteFile(pub, []byte(strings.Repeat("f", 1024)), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadPublicKey(pub); err == nil {
		t.Fatal("oversized public key accepted")
	}
}
