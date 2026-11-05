package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/arcvpn/arcvpn/internal/db"
	"github.com/arcvpn/arcvpn/internal/usage"
)

func usageFixture(t *testing.T) (*Store, User) {
	t.Helper()
	s, done := createControlTestStore(t)
	t.Cleanup(done)
	u := User{Name: "usage-user", UUID: "11111111-1111-4111-8111-111111111111", Subscription: "fixture-subscription", Enabled: true, CreatedAt: time.Now().Unix()}
	if err := s.InsertUser(context.Background(), u); err != nil {
		t.Fatal(err)
	}
	users, err := s.ListUsers(context.Background())
	if err != nil || len(users) != 1 {
		t.Fatal(err)
	}
	return s, users[0]
}

func BenchmarkUserAccess10000Users(b *testing.B) {
	database, err := db.Open(filepath.Join(b.TempDir(), "panel.sqlite"))
	if err != nil {
		b.Fatal(err)
	}
	defer database.Close()
	s := &Store{DB: database}
	ctx := context.Background()
	if err := s.Migrate(ctx); err != nil {
		b.Fatal(err)
	}
	if err := database.WithTx(ctx, func(tx *db.Tx) error {
		for i := 1; i <= 10000; i++ {
			if err := tx.Exec("INSERT INTO users(id,name,uuid,token,enabled,created_at) VALUES(?,?,?,?,1,1)", fmt.Sprint(i), fmt.Sprintf("user-%d", i), fmt.Sprintf("%08x-0000-4000-8000-%012x", i, i), fmt.Sprintf("fixture-token-%d", i)); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		allowed, _, err := s.UserAccess(ctx, 10000)
		if err != nil || !allowed {
			b.Fatal(allowed, err)
		}
	}
}
func TestUsageLedgerIsAtomicIdempotentAndResetPreservesCheckpoint(t *testing.T) {
	s, u := usageFixture(t)
	ctx := context.Background()
	now := time.Now().Unix()
	c := usage.Counter{UserID: u.ID, Identity: u.UUID, Stream: "vless-reality", Epoch: "epoch-a", StartedAt: 1, ObservedAt: now - 10, Upload: 100, Download: 200}
	if err := s.RecordUsage(ctx, []usage.Counter{c}, c.ObservedAt); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordUsage(ctx, []usage.Counter{c}, c.ObservedAt); err != nil {
		t.Fatal(err)
	}
	p, _ := s.Packages(ctx)
	if p[u.ID].Upload+p[u.ID].Download != 300 {
		t.Fatal("duplicate debit")
	}
	if err := s.SavePackage(ctx, u.ID, 1000, 0, 0, true); err != nil {
		t.Fatal(err)
	}
	c.ObservedAt = now - 5
	c.Upload = 150
	c.Download = 250
	if err := s.RecordUsage(ctx, []usage.Counter{c}, c.ObservedAt); err != nil {
		t.Fatal(err)
	}
	p, _ = s.Packages(ctx)
	if p[u.ID].Upload+p[u.ID].Download != 100 || p[u.ID].Period != 2 {
		t.Fatal("reset replayed old traffic", p[u.ID])
	}
	if err := s.SavePackage(ctx, u.ID, 1000, 0, 0, true); !errors.Is(err, ErrConflict) {
		t.Fatal("retry reset twice", err)
	}
	bad := c
	bad.ObservedAt = now
	bad.Upload = 1
	if err := s.RecordUsage(ctx, []usage.Counter{bad}, bad.ObservedAt); err == nil {
		t.Fatal("regression accepted")
	}
	p, _ = s.Packages(ctx)
	if p[u.ID].Upload+p[u.ID].Download != 100 {
		t.Fatal("partial debit")
	}
	if err := s.DB.WithTx(ctx, func(tx *db.Tx) error {
		return tx.Exec("CREATE TRIGGER reject_debit BEFORE INSERT ON traffic_debits BEGIN SELECT RAISE(ABORT,'fixture failure'); END")
	}); err != nil {
		t.Fatal(err)
	}
	c.ObservedAt = now
	c.Upload += 20
	if err := s.RecordUsage(ctx, []usage.Counter{c}, now); err == nil {
		t.Fatal("debit persistence failure ignored")
	}
	p, _ = s.Packages(ctx)
	if p[u.ID].Upload+p[u.ID].Download != 100 {
		t.Fatal("checkpoint or totals escaped rollback")
	}
}
func TestPortableAndLegacyRestoreNeverEraseKnownUsage(t *testing.T) {
	s, u := usageFixture(t)
	ctx := context.Background()
	now := time.Now().Unix()
	c := usage.Counter{UserID: u.ID, Identity: u.UUID, Stream: "vless-reality", Epoch: "a", StartedAt: 1, ObservedAt: now - 5, Upload: 100, Download: 200}
	if err := s.RecordUsage(ctx, []usage.Counter{c}, c.ObservedAt); err != nil {
		t.Fatal(err)
	}
	archive, err := s.UsageBackup(ctx)
	if err != nil {
		t.Fatal(err)
	}
	c.Upload = 200
	c.ObservedAt = now
	if err := s.RecordUsage(ctx, []usage.Counter{c}, now); err != nil {
		t.Fatal(err)
	}
	for _, candidate := range []*usage.State{&archive, nil} {
		if err := s.ReplaceFullStateWithUsage(ctx, nil, []User{u}, nil, nil, nil, nil, nil, nil, candidate); err != nil {
			t.Fatal(err)
		}
		p, _ := s.Packages(ctx)
		if p[u.ID].Upload+p[u.ID].Download != 400 {
			t.Fatal("restore erased spent bytes", p[u.ID])
		}
		if err := s.RecordUsage(ctx, []usage.Counter{c}, now); err != nil {
			t.Fatal(err)
		}
		p, _ = s.Packages(ctx)
		if p[u.ID].Upload+p[u.ID].Download != 400 {
			t.Fatal("restore replayed checkpoint")
		}
	}
}
func TestUsageBatchRejectsForeignIdentityAndHasNoCredentialLeak(t *testing.T) {
	s, u := usageFixture(t)
	ctx := context.Background()
	now := time.Now().Unix()
	c := usage.Counter{UserID: u.ID, Identity: "22222222-2222-4222-8222-222222222222", Stream: "vless-reality", Epoch: "a", StartedAt: 1, ObservedAt: now}
	if err := s.RecordUsage(ctx, []usage.Counter{c}, now); err == nil {
		t.Fatal("foreign credential accepted")
	}
	p, _ := s.Packages(ctx)
	if p[u.ID].ObservedAt != 0 {
		t.Fatal("unknown became zero")
	}
	blob, _ := json.Marshal(p)
	if strings.Contains(string(blob), u.UUID) || strings.Contains(string(blob), u.Subscription) {
		t.Fatal("public package leaked credentials")
	}
}

func TestControlledAndUnexpectedRestartsHaveDifferentReviewState(t *testing.T) {
	s, u := usageFixture(t)
	ctx := context.Background()
	now := time.Now().Unix()
	c := usage.Counter{UserID: u.ID, Identity: u.UUID, Stream: "vless-reality", Epoch: "a", StartedAt: 1, ObservedAt: now - 15, Upload: 100}
	if err := s.RecordUsage(ctx, []usage.Counter{c}, c.ObservedAt); err != nil {
		t.Fatal(err)
	}
	c.Epoch = "b"
	c.StartedAt = 2
	c.ObservedAt = now - 5
	c.Upload = 20
	if err := s.RecordUsageAfterApply(ctx, []usage.Counter{c}, c.ObservedAt, "a"); err != nil {
		t.Fatal(err)
	}
	items, _ := s.Packages(ctx)
	if !items[u.ID].Incomplete || items[u.ID].NeedsReview || items[u.ID].Upload != 120 {
		t.Fatal("controlled transition mishandled", items[u.ID])
	}
	c.Epoch = "c"
	c.StartedAt = 3
	c.ObservedAt = now
	c.Upload = 5
	if err := s.RecordUsage(ctx, []usage.Counter{c}, now); err != nil {
		t.Fatal(err)
	}
	items, _ = s.Packages(ctx)
	if !items[u.ID].NeedsReview || items[u.ID].Upload != 125 {
		t.Fatal("unplanned restart silently accepted")
	}
	if err := s.SavePackage(ctx, u.ID, 1000, 0, 0, false); err != nil {
		t.Fatal(err)
	}
	items, _ = s.Packages(ctx)
	if items[u.ID].Reason(now, true) != "accounting_gap" {
		t.Fatal("uncertain finite package remained open")
	}
	if err := s.SavePackage(ctx, u.ID, 1000, 0, 1, true); err != nil {
		t.Fatal(err)
	}
	items, _ = s.Packages(ctx)
	if items[u.ID].NeedsReview || items[u.ID].Upload != 0 {
		t.Fatal("explicit new period did not clear review")
	}
}
