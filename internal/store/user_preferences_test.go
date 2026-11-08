package store

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestUserPreferencesCASAndIdentity(t *testing.T) {
	s, u := usageFixture(t)
	ctx := context.Background()
	old, e := s.ByID(ctx, u.ID)
	if e != nil {
		t.Fatal(e)
	}
	p := UserPreferences{Group: "Team", Title: "My VPN", AllowedKinds: []string{}}
	if e = s.SaveUserPreferences(ctx, u.ID, "Renamed", p, 0); e != nil {
		t.Fatal(e)
	}
	got, e := s.ByID(ctx, u.ID)
	if e != nil {
		t.Fatal(e)
	}
	if got.Name != "Renamed" || got.UUID != old.UUID || got.Subscription != old.Subscription || got.Preferences.Version != 1 || got.Allows("wireguard") || !reflect.DeepEqual(got.Preferences.AllowedKinds, []string{}) {
		t.Fatal("identity or policy changed incorrectly", got.Preferences)
	}
	if err := s.DeleteUser(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.RestoreUser(ctx, got); err != nil {
		t.Fatal(err)
	}
	restored, _ := s.ByID(ctx, u.ID)
	if restored.Allows("wireguard") || restored.UUID != old.UUID || restored.Preferences.Version != 1 {
		t.Fatal("restore widened assignments")
	}
	if e = s.SaveUserPreferences(ctx, u.ID, "Stale", p, 0); !errors.Is(e, ErrConflict) {
		t.Fatal(e)
	}
	if _, e = s.CreateEnrollment(ctx, "node"); !errors.Is(e, ErrUserAssignmentsUnavailable) {
		t.Fatal("enrollment bypass", e)
	}
	p.AllowedKinds = nil
	if e = s.SaveUserPreferences(ctx, u.ID, "Renamed", p, 1); e != nil {
		t.Fatal(e)
	}
	if _, e = s.CreateEnrollment(ctx, "node"); e != nil {
		t.Fatal(e)
	}
	p.AllowedKinds = []string{"wireguard"}
	if e = s.SaveUserPreferences(ctx, u.ID, "Renamed", p, 2); !errors.Is(e, ErrUserAssignmentsUnavailable) {
		t.Fatal("pending enrollment bypass", e)
	}
}
func TestUserPreferencesValidation(t *testing.T) {
	for _, p := range []UserPreferences{{AllowedKinds: []string{"invalid"}}, {AllowedKinds: []string{"wireguard", "wireguard"}}, {Title: "bad\nheader"}, {AccessMode: "direct"}, {AccessMode: "direct", DirectKind: "wireguard", AllowedKinds: []string{"hysteria2"}}} {
		if p.Valid() {
			t.Fatalf("accepted invalid %#v", p)
		}
	}
}
