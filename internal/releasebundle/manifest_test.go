package releasebundle

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSignedInstallerAuthenticatesArchiveMetadataAndSigner(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "arcvpn-0.7.0-stage5-a2-linux-amd64.tar.gz")
	if err := os.WriteFile(path, []byte("synthetic archive bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	public, private, _ := ed25519.GenerateKey(rand.Reader)
	body, sig, err := Sign(path, "0.7.0-stage5-a2", strings.Repeat("a", 64), private)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(path, body, sig, public); err != nil {
		t.Fatal(err)
	}
	other, _, _ := ed25519.GenerateKey(rand.Reader)
	if _, err := Verify(path, body, sig, other); err == nil {
		t.Fatal("wrong signer accepted")
	}
	if _, err := Verify(path, append(body, ' '), sig, public); err == nil {
		t.Fatal("changed metadata accepted")
	}
	// A signature from this protocol cannot authorize an updater manifest.
	if ed25519.Verify(public, body, sig) {
		t.Fatal("signature domain separation missing")
	}
	if err := os.WriteFile(path, []byte("changed archive bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(path, body, sig, public); err == nil {
		t.Fatal("tampered archive accepted")
	}
	if _, _, err := Sign(path, "../../escape", strings.Repeat("a", 64), private); err == nil {
		t.Fatal("unsafe version accepted")
	}
}

func TestOfflineKeysArePrivateAndNeverOverwritten(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "signer")
	if err := Keygen(dir); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "installer-private.hex")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := Keygen(dir); err == nil {
		t.Fatal("existing signer replaced")
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("private key mutated")
	}
	private, err := ReadKey(path, true)
	if err != nil {
		t.Fatal(err)
	}
	public, err := ReadKey(filepath.Join(dir, "installer-public.hex"), false)
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(ed25519.PrivateKey(private).Public().(ed25519.PublicKey)) != hex.EncodeToString(public) {
		t.Fatal("keypair mismatch")
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadKey(path, true); err == nil {
		t.Fatal("public-readable private key accepted")
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadKey(link, false); err == nil {
		t.Fatal("key symlink accepted")
	}
}
