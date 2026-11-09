package app

import (
	"context"
	"testing"
)

func TestUserSnapshotNeverConfirmsDatabaseOnlyOrUnverifiedRuntime(t *testing.T) {
	a := firstProtocolApp(t)
	ctx := context.Background()
	check := func(want bool) {
		t.Helper()
		_, got, err := a.UsersSnapshot(ctx)
		if err != nil || got != want {
			t.Fatalf("confirmed=%v want=%v err=%v", got, want, err)
		}
	}
	check(false)
	a.usageApplied = "fixture-applied"
	if err := a.Store.SetAccountingState(ctx, "applied"); err != nil {
		t.Fatal(err)
	}
	check(false) // A skipped restart cannot establish runtime health.
	a.Core.SkipRestart = false
	a.Engines.SkipRestart = false
	check(true)
	a.runtimeUncertain = true
	check(false)
	a.runtimeUncertain = false
	if err := a.Store.SetAccountingState(ctx, "apply_failed"); err != nil {
		t.Fatal(err)
	}
	check(false)
}
