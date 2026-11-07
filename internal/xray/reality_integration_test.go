package xray

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/arcvpn/arcvpn/internal/store"
)

func TestRealRealityConfiguration(t *testing.T) {
	binary := os.Getenv("ARCVPN_TEST_XRAY")
	if binary == "" {
		t.Skip("set ARCVPN_TEST_XRAY to the pinned test binary")
	}
	private, public, shortID, err := GenerateKeys(context.Background(), binary)
	if err != nil {
		t.Fatal(err)
	}
	c := Controller{Binary: binary, Path: filepath.Join(t.TempDir(), "xray.json"), SkipRestart: true}
	reality := store.Reality{Address: "1.1.1.1", Port: 443, Target: "www.microsoft.com", SNI: "www.microsoft.com", Private: private, Public: public, ShortID: shortID, Enabled: true}
	if err = c.ApplyWithProfiles(context.Background(), reality, []store.User{{ID: 1, Name: "fixture", UUID: "11111111-1111-4111-8111-111111111111", Enabled: true}}, nil); err != nil {
		t.Fatal(err)
	}
}
