package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/arcvpn/arcvpn/internal/updater"
	"os"
	"path/filepath"
	"testing"
)

func TestReleasePreparationDoesNotRotateTrustOrOverwrite(t *testing.T) {
	core := os.Getenv("ARCVPN_TEST_RELEASE_CORE")
	if core == "" {
		t.Skip("requires trusted build fixture")
	}
	pub, key, _ := ed25519.GenerateKey(rand.Reader)
	dir := t.TempDir()
	secret := filepath.Join(dir, "private.hex")
	public := filepath.Join(dir, "public.hex")
	notes := filepath.Join(dir, "notes.md")
	source := filepath.Join(dir, "source.json")
	out := filepath.Join(dir, "release")
	for name, value := range map[string][]byte{secret: []byte(hex.EncodeToString(key)), public: []byte(hex.EncodeToString(pub)), notes: []byte("Test release; no production acceptance"), source: []byte(`{"format":"arcnet-update-catalog-v1","releases":[]}`)} {
		if err := os.WriteFile(name, value, 0600); err != nil {
			t.Fatal(err)
		}
	}
	binaryData, _ := os.ReadFile(core)
	sum := sha256.Sum256(binaryData)
	receipt := buildReceipt{Format: "arcvpn-build-v1", Version: "0.7.0-stage5-a10", Target: "linux/amd64", CoreSHA256: hex.EncodeToString(sum[:]), FrontendVerified: true}
	receiptRaw, _ := json.Marshal(receipt)
	receiptPath := filepath.Join(dir, "BUILDINFO.json")
	os.WriteFile(receiptPath, receiptRaw, 0600)
	args := []string{"--binary", core, "--build-info", receiptPath, "--version", "0.7.0-stage5-a10", "--minimum-version", "0.7.0-stage5-a7", "--key", secret, "--public-key", public, "--catalog-source", source, "--notes", notes, "--out-dir", out}
	if err := prepareUpdate(args); err != nil {
		t.Fatal(err)
	}
	blob, _ := os.ReadFile(filepath.Join(out, "update-a10.zip"))
	signed, _ := os.ReadFile(filepath.Join(out, "update-catalog.json"))
	if _, err := updater.VerifyPublished(blob, signed, pub, "0.7.0-stage5-a7", "linux/amd64"); err != nil {
		t.Fatal(err)
	}
	if err := prepareUpdate(args); err == nil {
		t.Fatal("output overwritten")
	}
	raw, _ := os.ReadFile(filepath.Join(out, "catalog-source.json"))
	var c updater.Catalog
	if json.Unmarshal(raw, &c) != nil || len(c.Releases) != 1 {
		t.Fatal("catalog incomplete")
	}
	other, _, _ := ed25519.GenerateKey(rand.Reader)
	os.WriteFile(public, []byte(hex.EncodeToString(other)), 0600)
	if _, _, err := signingKey(secret, public); err == nil {
		t.Fatal("foreign signer accepted")
	}
	data, _ := os.ReadFile(core)
	if validateReleaseBinary(data, "0.7.1", receipt) == nil {
		t.Fatal("wrong version stamp accepted")
	}
	for _, mutate := range []func(*buildReceipt){
		func(r *buildReceipt) { r.FrontendVerified = false },
		func(r *buildReceipt) { r.CoreSHA256 = "invalid" },
		func(r *buildReceipt) { r.Target = "darwin/arm64" },
	} {
		invalid := receipt
		mutate(&invalid)
		if validateReleaseBinary(data, receipt.Version, invalid) == nil {
			t.Fatal("unverified build receipt accepted")
		}
	}
}
