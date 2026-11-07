package store

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/arcvpn/arcvpn/internal/auth"
)

func TestAdminSetupExpiresRotatesAndHasSingleOwner(t *testing.T) {
	s, done := createControlTestStore(t)
	defer done()
	ctx := context.Background()
	code := auth.HashToken("one-time-code")
	if err := s.BeginAdminSetup(ctx, code, time.Now().Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteAdminSetup(ctx, code, "owner", "hash"); err == nil {
		t.Fatal("expired code accepted")
	}
	if err := s.BeginAdminSetup(ctx, code, time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteAdminSetup(ctx, auth.HashToken("wrong"), "owner", "hash"); err == nil {
		t.Fatal("wrong code accepted")
	}
	var wins atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if s.CompleteAdminSetup(ctx, code, "owner", "hash") == nil {
				wins.Add(1)
			}
		}()
	}
	wg.Wait()
	if wins.Load() != 1 {
		t.Fatalf("owners: %d", wins.Load())
	}
	if ok, err := s.AdminSetupAvailable(ctx); err != nil || ok {
		t.Fatal("setup remains open", err)
	}
	if err := s.BeginAdminSetup(ctx, code, time.Now().Add(time.Minute)); err == nil {
		t.Fatal("setup reopened over existing owner")
	}
	if err := s.CompleteAdminSetup(ctx, code, "attacker", "hash"); err == nil {
		t.Fatal("code replay")
	}
}

func TestChangeAdminCredentialsRevokesSessionsAndUsesCAS(t *testing.T) {
	s, done := createControlTestStore(t)
	defer done()
	ctx := context.Background()
	if err := s.CreateAdmin(ctx, "owner", "old-hash"); err != nil {
		t.Fatal(err)
	}
	if err := s.AddSession(ctx, "session", "csrf", time.Now().Add(time.Hour).Unix()); err != nil {
		t.Fatal(err)
	}
	if err := s.ChangeAdminCredentials(ctx, "stale-hash", "changed", "new-hash"); err == nil {
		t.Fatal("stale reauthentication accepted")
	}
	if err := s.ChangeAdminCredentials(ctx, "old-hash", "changed", "new-hash"); err != nil {
		t.Fatal(err)
	}
	if _, valid, err := s.Session(ctx, "session"); err != nil || valid {
		t.Fatal("old session remains valid", err)
	}
	if name, err := s.AdminUsername(ctx); err != nil || name != "changed" {
		t.Fatal(name, err)
	}
}
