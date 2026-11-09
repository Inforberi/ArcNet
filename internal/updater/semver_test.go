package updater

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestSemVerCatalogUsesSameRootTrustAndRejectsDowngrade(t *testing.T) {
	pub, key, _ := ed25519.GenerateKey(rand.Reader)
	blob, err := MakePackage([]byte(strings.Repeat("trusted binary", 200)), "0.7.1", key)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(blob)
	c := Catalog{Format: "arcnet-update-catalog-v2", Releases: []Release{{Version: "0.7.1", MinimumVersion: "0.7.0-stage5-a10", Platform: "linux/amd64", Scope: "core-ui", File: "update-0.7.1.zip", Size: int64(len(blob)), SHA256: hex.EncodeToString(hash[:])}}}
	raw, _ := json.Marshal(c)
	signed, err := SignCatalog(raw, key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = VerifyPublished(blob, signed, pub, "0.7.0-stage5-a10", "linux/amd64"); err != nil {
		t.Fatal("bridge upgrade", err)
	}
	for _, current := range []string{"0.7.0-stage5-a9", "0.7.1", "0.7.2", "1.0.0", "unknown"} {
		if _, err = VerifyPublished(blob, signed, pub, current, "linux/amd64"); err == nil {
			t.Fatal("unsafe transition", current)
		}
	}
	if _, err = VerifyPublished(blob, signed, pub, "0.7.0-stage5-a10", "linux/arm64"); err == nil {
		t.Fatal("wrong platform")
	}
	c.Format = "arcnet-update-catalog-v1"
	raw, _ = json.Marshal(c)
	if _, err = SignCatalog(raw, key); err == nil {
		t.Fatal("v1 accepts incompatible new schema")
	}
	c.Format = "arcnet-update-catalog-v2"
	c.Releases[0].PublishedAt = "invented"
	raw, _ = json.Marshal(c)
	if _, err = SignCatalog(raw, key); err == nil {
		t.Fatal("invalid date")
	}
}
func TestHistoryDoesNotGrantInstallationAuthority(t *testing.T) {
	rows, err := parseHistory([]byte(`[{"tag_name":"v0.7.1","published_at":"2026-11-09T12:00:00Z","body":"Fixes"},{"tag_name":"v0.7.0-stage5-a9","body":"Older"},{"tag_name":"../../../shell","body":"Invalid"},{"tag_name":"v0.8.0","draft":true}]`))
	if err != nil || len(rows) != 2 || rows[0].Version != "0.7.1" || rows[1].PublishedAt != "" {
		t.Fatal(rows, err)
	}
	raw, _ := json.Marshal(rows)
	if strings.Contains(string(raw), "installable") || strings.Contains(string(raw), "url") {
		t.Fatal("history acquired install capability")
	}
	if _, err = parseHistory([]byte(`{"message":"rate limited"}`)); err == nil {
		t.Fatal("API error mistaken for history")
	}
}

func TestNewCatalogFallbackNeverHidesSignatureFailure(t *testing.T) {
	c, _, pub, key := catalogFixture(t)
	raw, _ := json.Marshal(c)
	signed, _ := SignCatalog(raw, key)
	calls := 0
	fetch := func(_ context.Context, url string, _ int64) ([]byte, error) {
		calls++
		if url == CatalogV2URL {
			return []byte("bad signature"), nil
		}
		return signed, nil
	}
	if _, err := fetchCatalog(context.Background(), pub, fetch); err == nil || calls != 1 {
		t.Fatal("signature failure fell back", calls, err)
	}
	calls = 0
	fetch = func(_ context.Context, url string, _ int64) ([]byte, error) {
		calls++
		if url == CatalogV2URL {
			return nil, errors.New("not published")
		}
		return signed, nil
	}
	got, err := fetchCatalog(context.Background(), pub, fetch)
	if err != nil || !got.Partial || calls != 2 {
		t.Fatal("verified legacy fallback", err, calls)
	}
}
