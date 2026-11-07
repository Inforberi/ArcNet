package store

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/arcvpn/arcvpn/internal/db"
)

func TestFingerprintPersistenceRestoreAndRollback(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	s := &Store{DB: database}
	ctx := context.Background()
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	r := Reality{Address: "198.51.100.1", Port: 443, Fingerprint: "firefox"}
	p := Protocol{Kind: "vless-xhttp", Address: r.Address, Port: 8444, Fingerprint: "safari"}
	if err := s.SaveReality(ctx, r); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertProtocol(ctx, p); err != nil {
		t.Fatal(err)
	}
	check := func(real, extra string) {
		t.Helper()
		got, err := s.ReadReality(ctx)
		if err != nil || got.Fingerprint != real {
			t.Fatal("REALITY metadata", err, got.Fingerprint)
		}
		profile, err := s.GetProtocol(ctx, p.Kind)
		if err != nil || profile.Fingerprint != extra {
			t.Fatal("XHTTP metadata", err, profile.Fingerprint)
		}
		all, err := s.ListProtocols(ctx)
		if err != nil || len(all) != 1 || all[0].Fingerprint != extra {
			t.Fatal("profile list", err)
		}
	}
	check("firefox", "safari")
	p.Fingerprint = "edge"
	if err := s.ReplaceProtocols(ctx, []Protocol{p}, []ProtocolCredential{{UserID: 999, Kind: p.Kind, Secret: "fixture"}}); err == nil {
		t.Fatal("invalid restore accepted")
	}
	check("firefox", "safari")
	r.Fingerprint = "ios"
	if err := s.ReplaceFullState(ctx, &r, nil, []Protocol{p}, nil); err != nil {
		t.Fatal(err)
	}
	check("ios", "edge")
	r.Fingerprint = ""
	p.Fingerprint = ""
	if err := s.ReplaceFullState(ctx, &r, nil, []Protocol{p}, nil); err != nil {
		t.Fatal(err)
	}
	check("", "")
}
