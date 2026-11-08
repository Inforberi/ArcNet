package store

import (
	"context"
	"errors"
	"testing"
)

func TestUserBatchEditAtomic(t *testing.T) {
	s, u := usageFixture(t)
	ctx := context.Background()
	group := "Team"
	quota := int64(500)
	expiry := int64(2000000000)
	in := UserBatchChanges{Group: &group, QuotaBytes: &quota, ExpiresAt: &expiry, Connections: &UserBatchConnections{AllowedKinds: []string{"wireguard"}}}
	items := []UserBatchSelection{{UserSelection: UserSelection{ID: u.ID, ExpectedEnabled: true}}, {UserSelection: UserSelection{ID: 9999}}}
	if err := s.BulkEditUsers(ctx, items, in); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	got, p, _ := s.UserEditor(ctx, u.ID)
	if got.Preferences.Version != 0 || p.QuotaBytes != 0 {
		t.Fatal("partial bulk save")
	}
	if err := s.BulkEditUsers(ctx, items[:1], in); err != nil {
		t.Fatal(err)
	}
	got, p, _ = s.UserEditor(ctx, u.ID)
	if got.Preferences.Group != group || got.Allows("vless-reality") || !got.Allows("wireguard") || p.QuotaBytes != quota || p.ExpiresAt != expiry {
		t.Fatal("wrong bulk policy")
	}
	if err := s.BulkEditUsers(ctx, items[:1], in); !errors.Is(err, ErrConflict) {
		t.Fatal("stale batch", err)
	}
	if err := s.BulkEditUsers(ctx, items[:1], UserBatchChanges{}); !errors.Is(err, ErrInvalidUserPreferences) {
		t.Fatal("empty changes", err)
	}
}
func TestUserBatchCreateAtomicCredentials(t *testing.T) {
	s, _ := usageFixture(t)
	ctx := context.Background()
	initial, _ := s.ListUsers(ctx)
	seeds := []User{{Name: "New one", UUID: "new-one", Subscription: "new-one", Enabled: true}, {Name: "New two", UUID: "new-two", Subscription: "new-two", Enabled: true}}
	called := 0
	_, err := s.CreateUsers(ctx, seeds, func(u User) ([]ProtocolCredential, error) {
		called++
		if called == 2 {
			return nil, errors.New("generator failed")
		}
		return nil, nil
	})
	if err == nil {
		t.Fatal("accepted failed key generation")
	}
	got, _ := s.ListUsers(ctx)
	if len(got) != len(initial) {
		t.Fatal("partial creation")
	}
	seeds[1].Name = seeds[0].Name
	if _, err = s.CreateUsers(ctx, seeds, func(User) ([]ProtocolCredential, error) { return nil, nil }); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	got, _ = s.ListUsers(ctx)
	if len(got) != len(initial) {
		t.Fatal("duplicate left partial batch")
	}
}
