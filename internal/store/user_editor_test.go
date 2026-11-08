package store

import (
	"context"
	"errors"
	"testing"
)

func TestUserEditorAtomicAndBulk(t *testing.T) {
	s, u := usageFixture(t)
	ctx := context.Background()
	original, p, err := s.UserEditor(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	in := UserEdit{Name: "Unified", Preferences: original.Preferences, ExpectedEnabled: original.Enabled, Enabled: false, Package: &UserPackageEdit{QuotaBytes: 1000, ExpectedVersion: p.Version + 1}}
	if err = s.SaveUserEditor(ctx, u.ID, in, true); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	got, _, _ := s.UserEditor(ctx, u.ID)
	if got.Name != original.Name || got.Enabled != original.Enabled || got.Preferences.Version != 0 {
		t.Fatal("partial editor save")
	}
	in.Package.ExpectedVersion = p.Version
	if err = s.SaveUserEditor(ctx, u.ID, in, true); err != nil {
		t.Fatal(err)
	}
	got, p, err = s.UserEditor(ctx, u.ID)
	if err != nil || got.Name != "Unified" || got.Enabled || p.QuotaBytes != 1000 || p.Version != 1 || got.UUID != original.UUID || got.Subscription != original.Subscription {
		t.Fatal("incorrect unified save", err)
	}
	items := []UserSelection{{u.ID, got.Preferences.Version, false}, {9999, 0, false}}
	if err = s.BulkUserAccess(ctx, items, true); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	got, _, _ = s.UserEditor(ctx, u.ID)
	if got.Enabled || got.Preferences.Version != 1 {
		t.Fatal("partial bulk save")
	}
	if err = s.BulkUserAccess(ctx, items[:1], true); err != nil {
		t.Fatal(err)
	}
	if err = s.BulkUserAccess(ctx, items[:1], false); !errors.Is(err, ErrConflict) {
		t.Fatal("stale bulk accepted", err)
	}
	if err = s.BulkUserAccess(ctx, []UserSelection{{u.ID, 2, true}, {u.ID, 2, true}}, false); !errors.Is(err, ErrInvalidUserPreferences) {
		t.Fatal("duplicate accepted", err)
	}
	if err = s.BulkUserAccess(ctx, make([]UserSelection, 101), false); !errors.Is(err, ErrInvalidUserPreferences) {
		t.Fatal("unbounded bulk", err)
	}
	got, p, _ = s.UserEditor(ctx, u.ID)
	if !got.Enabled || p.QuotaBytes != 1000 || got.Preferences.Version != 2 {
		t.Fatal("bulk lost package or revisions")
	}
}
