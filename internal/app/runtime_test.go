package app

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/arcvpn/arcvpn/internal/lifecycle"
	"github.com/arcvpn/arcvpn/internal/usage"
)

func TestRuntimePauseBlocksApplyAndUserMutationRollsBack(t *testing.T) {
	a := firstProtocolApp(t)
	ctx := context.Background()
	if err := a.Setup(ctx, "198.51.100.25", "www.example.com", 443); err != nil {
		t.Fatal(err)
	}
	u, err := a.AddUser(ctx, "User")
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(a.Core.Path)
	if err != nil {
		t.Fatal(err)
	}
	a.RuntimeCheck = func() error { return lifecycle.ErrPaused }
	if err := a.ToggleUser(ctx, u.ID, false); !errors.Is(err, lifecycle.ErrPaused) {
		t.Fatal(err)
	}
	after, err := os.ReadFile(a.Core.Path)
	if err != nil || string(before) != string(after) {
		t.Fatal("paused apply changed config", err)
	}
	user, err := a.Store.ByID(ctx, u.ID)
	if err != nil || !user.Enabled || user.UUID != u.UUID || user.Subscription != u.Subscription {
		t.Fatal("paused toggle lost user state", err)
	}
}

func TestFinalUsageFailurePreservesKnownTotalsAndReviewGate(t *testing.T) {
	a := firstProtocolApp(t)
	ctx := context.Background()
	if err := a.Setup(ctx, "198.51.100.25", "www.example.com", 443); err != nil {
		t.Fatal(err)
	}
	u, err := a.AddUser(ctx, "User")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().Unix()
	first := usage.Counter{UserID: u.ID, Identity: u.UUID, Stream: "vless-reality", Epoch: "first", StartedAt: 1, ObservedAt: now - 2, Upload: 100, Download: 200}
	if err := a.Store.RecordUsage(ctx, []usage.Counter{first}, first.ObservedAt); err != nil {
		t.Fatal(err)
	}
	next := first
	next.Epoch = "unexpected"
	next.StartedAt = now - 1
	next.ObservedAt = now - 1
	next.Upload = 1
	next.Download = 2
	if err := a.Store.RecordUsage(ctx, []usage.Counter{next}, next.ObservedAt); err != nil {
		t.Fatal(err)
	}
	before, err := a.Store.Package(ctx, u.ID)
	if err != nil || !before.NeedsReview {
		t.Fatal("fixture missing review gate", err)
	}
	if err := a.FinalLocalUsage(ctx); err == nil {
		t.Fatal("missing private statistics became confirmed zero")
	}
	after, err := a.Store.Package(ctx, u.ID)
	if err != nil || after != before {
		t.Fatal("failed final observation reset accounting", after, err)
	}
	user, err := a.Store.ByID(ctx, u.ID)
	if err != nil || user.Subscription != u.Subscription || user.UUID != u.UUID {
		t.Fatal("CLI accounting rotated access", err)
	}
}
