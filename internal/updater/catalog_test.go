package updater

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func catalogFixture(t *testing.T) (Catalog, []byte, ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, key, _ := ed25519.GenerateKey(rand.Reader)
	data, err := MakePackage([]byte(strings.Repeat("binary", 1000)), "0.7.0-stage5-a8", key)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(data)
	c := Catalog{Format: "arcnet-update-catalog-v1", Releases: []Release{{Version: "0.7.0-stage5-a8", MinimumVersion: "0.7.0-stage5-a5", Platform: "linux/amd64", Scope: "core-ui", File: "update-a8.zip", Size: int64(len(data)), SHA256: hex.EncodeToString(hash[:]), Notes: "UI update"}}}
	return c, data, pub, key
}
func TestSignedUpdateCatalogAndCompatibility(t *testing.T) {
	c, data, pub, key := catalogFixture(t)
	raw, _ := json.Marshal(c)
	signed, err := SignCatalog(raw, key)
	if err != nil {
		t.Fatal(err)
	}
	got, err := VerifyCatalog(signed, pub)
	if err != nil || len(got.Releases) != 1 {
		t.Fatal(err)
	}
	r := got.Releases[0]
	for _, test := range []struct {
		current, platform string
		allowed           bool
	}{{"0.7.0-stage5-a7", "linux/amd64", true}, {"0.7.0-stage5-a8", "linux/amd64", false}, {"0.7.0-stage5-a9", "linux/amd64", false}, {"0.7.0-stage5-a4", "linux/amd64", false}, {"0.7.0-stage5-a7", "linux/arm64", false}, {"0.8.0", "linux/amd64", false}} {
		if r.Compatible(test.current, test.platform) != test.allowed {
			t.Fatal("compatibility", test)
		}
	}
	if _, err = verifyDownload(data, r, pub); err != nil {
		t.Fatal(err)
	}
	broken := append([]byte(nil), data...)
	broken[0] ^= 1
	if _, err = verifyDownload(broken, r, pub); err == nil {
		t.Fatal("tampered ZIP accepted")
	}
	r.Version = "0.7.0-stage5-a9"
	if _, err = verifyDownload(data, r, pub); err == nil {
		t.Fatal("wrong package version accepted")
	}
	other, _, _ := ed25519.GenerateKey(rand.Reader)
	if _, err = VerifyCatalog(signed, other); err == nil {
		t.Fatal("foreign catalog signer accepted")
	}
	signed[len(signed)/2] ^= 1
	if _, err = VerifyCatalog(signed, pub); err == nil {
		t.Fatal("tampered catalog accepted")
	}
}
func TestUpdateCatalogRejectsUnsafeMetadata(t *testing.T) {
	base, _, _, key := catalogFixture(t)
	for _, change := range []func(*Catalog){
		func(c *Catalog) { c.Releases = append(c.Releases, c.Releases[0]) },
		func(c *Catalog) { c.Releases[0].File = "../../payload.zip" },
		func(c *Catalog) { c.Releases[0].Platform = "windows/amd64" },
		func(c *Catalog) { c.Releases[0].Size = MaxArchive + 1 },
		func(c *Catalog) { c.Releases[0].Scope = "system" },
		func(c *Catalog) { c.Releases[0].MinimumVersion = "0.7.0-stage5-a9" },
	} {
		raw, _ := json.Marshal(base)
		var c Catalog
		json.Unmarshal(raw, &c)
		change(&c)
		raw, _ = json.Marshal(c)
		if _, err := SignCatalog(raw, key); err == nil {
			t.Fatal("unsafe metadata signed")
		}
	}
}

func TestUpdateRedirectBoundary(t *testing.T) {
	for _, tc := range []struct {
		url   string
		allow bool
	}{{"https://release-assets.githubusercontent.com/file", true}, {"https://github.com/Inforberi/ArcNet/releases", true}, {"http://github.com/file", false}, {"https://127.0.0.1/file", false}, {"https://github.com.evil.test/file", false}, {"https://evil.test/file", false}, {"https://github.com:443/file", false}} {
		req, _ := http.NewRequest("GET", tc.url, nil)
		if (releaseRedirect(req, nil) == nil) != tc.allow {
			t.Fatal("redirect policy", tc.url)
		}
	}
}

func TestPrivilegedInstallerRejectsReplay(t *testing.T) {
	c, data, pub, key := catalogFixture(t)
	dir := t.TempDir()
	public := filepath.Join(dir, "public.hex")
	staged := filepath.Join(dir, "staged.zip")
	if err := os.WriteFile(public, []byte(hex.EncodeToString(pub)), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(staged, data, 0600); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(c)
	signed, err := SignCatalog(raw, key)
	if err != nil {
		t.Fatal(err)
	}
	catalogPath := filepath.Join(dir, "catalog.json")
	if err = os.WriteFile(catalogPath, signed, 0600); err != nil {
		t.Fatal(err)
	}
	installer := Installer{PublicKey: public, Staged: staged, CurrentVersion: c.Releases[0].Version, Platform: "linux/amd64"}
	if _, err := installer.Verify(); err == nil {
		t.Fatal("root verifier accepted replay of installed version")
	}
	installer.CurrentVersion = "0.7.0-stage5-a7"
	if _, err := installer.Verify(); err != nil {
		t.Fatal("root verifier rejected forward update", err)
	}
	c.Releases[0].MinimumVersion = "0.7.0-stage5-a8"
	raw, _ = json.Marshal(c)
	signed, err = SignCatalog(raw, key)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(catalogPath, signed, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = installer.Verify(); err == nil {
		t.Fatal("root verifier bypassed signed minimum version")
	}
}
