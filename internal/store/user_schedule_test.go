package store

import (
	"context"
	"errors"
	"github.com/arcvpn/arcvpn/internal/db"
	"github.com/arcvpn/arcvpn/internal/usage"
	"testing"
	"time"
)

func TestScheduledResetFreshAndIdempotent(t *testing.T) {
	s, u := usageFixture(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Second)
	initial, pack, err := s.UserEditor(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	in := UserEdit{Name: initial.Name, Preferences: initial.Preferences, ExpectedEnabled: true, Enabled: true, Package: &UserPackageEdit{ExpectedVersion: pack.Version, ExpiresAt: now.Add(time.Hour).Unix(), Schedule: &usage.ResetSchedule{Frequency: "daily", Timezone: "UTC"}}}
	if err = s.SaveUserEditor(ctx, u.ID, in, true); err != nil {
		t.Fatal(err)
	}
	if _, err = s.CreateEnrollment(ctx, "blocked"); !errors.Is(err, ErrUsageUnsupported) {
		t.Fatal("schedule node gate", err)
	}
	err = s.DB.WithTx(ctx, func(tx *db.Tx) error {
		packages, e := readPackages(tx)
		if e != nil {
			return e
		}
		p := packages[u.ID]
		p.Upload = 40
		p.Download = 20
		p.ResetSchedule.NextAt = now.Add(-72 * time.Hour).Unix()
		return savePackage(tx, p)
	})
	if err != nil {
		t.Fatal(err)
	}
	if n, err := s.ResetDuePackages(ctx, now); err != nil || n != 0 {
		t.Fatal("reset with stale counters", n, err)
	}
	if err = s.SaveReality(ctx, Reality{Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err = s.RecordUsage(ctx, nil, now.Unix()); err != nil {
		t.Fatal(err)
	}
	if n, err := s.ResetDuePackages(ctx, now); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	_, p, _ := s.UserEditor(ctx, u.ID)
	if p.Upload != 0 || p.Download != 0 || p.Period != 2 || p.ExpiresAt != in.Package.ExpiresAt || p.ResetSchedule.NextAt <= now.Unix() {
		t.Fatal("incorrect catch-up reset", p)
	}
	if n, err := s.ResetDuePackages(ctx, now); err != nil || n != 0 {
		t.Fatal("double reset", n, err)
	}
	if n, err := s.ResetDuePackages(ctx, now.Add(-time.Hour)); err != nil || n != 0 {
		t.Fatal("clock rollback", n, err)
	}
}
