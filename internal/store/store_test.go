package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/arcvpn/arcvpn/internal/db"
)

func TestStore(t *testing.T) {
	ctx := context.Background()
	database, err := db.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	s := &Store{DB: database}
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateAdmin(ctx, "admin", "hash"); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateAdmin(ctx, "another", "hash"); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected conflict: %v", err)
	}
	if err := s.AddSession(ctx, "token_hash", "csrf", time.Now().Add(time.Hour).Unix()); err != nil {
		t.Fatal(err)
	}
	if csrf, valid, err := s.Session(ctx, "token_hash"); err != nil || !valid || csrf != "csrf" {
		t.Fatal("session invalid", err)
	}
	cfg := Reality{Address: "203.0.113.10", Port: 443, Target: "www.example.com", SNI: "www.example.com", Private: "private-key", Public: "public-key", ShortID: "0123456789abcdef", Enabled: true}
	if err := s.SaveReality(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	found, err := s.ReadReality(ctx)
	if err != nil || found.Port != 443 {
		t.Fatal(err)
	}
	u := User{Name: "test user", UUID: "c4d00000-0000-4000-8000-000000000000", Subscription: "token", Enabled: true, CreatedAt: 123}
	if err := s.InsertUser(ctx, u); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertUser(ctx, u); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate not prevented %v", err)
	}
	users, err := s.ListUsers(ctx)
	if err != nil || len(users) != 1 {
		t.Fatal("wrong number of users", err)
	}
	user := users[0]
	if err := s.SetEnabled(ctx, user.ID, false); err != nil {
		t.Fatal(err)
	}
	updated, err := s.ByID(ctx, user.ID)
	if err != nil || updated.Enabled {
		t.Fatal("not disabled", err)
	}
	if err := s.DeleteUser(ctx, user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ByID(ctx, user.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("deleted user found")
	}
	if err := s.RestoreUser(ctx, user); err != nil {
		t.Fatal(err)
	}
}
