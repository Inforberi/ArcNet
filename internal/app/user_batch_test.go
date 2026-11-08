package app

import (
	"context"
	"github.com/arcvpn/arcvpn/internal/protocols"
	"github.com/arcvpn/arcvpn/internal/store"
	"testing"
)

func TestUserBatchCreationAndMetadata(t *testing.T) {
	a := firstProtocolApp(t)
	ctx := context.Background()
	if err := a.Setup(ctx, "198.51.100.19", "www.example.com", 443); err != nil {
		t.Fatal(err)
	}
	if _, err := a.AddProtocol(ctx, protocols.Hysteria2, "198.51.100.19", 8444, "", ""); err != nil {
		t.Fatal(err)
	}
	users, err := a.CreateUsers(ctx, []string{"Alice", "Bob"}, "Team")
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 2 || users[0].ID == users[1].ID || users[0].UUID == users[1].UUID || users[0].Subscription == users[1].Subscription {
		t.Fatal("identities not unique")
	}
	for _, u := range users {
		credentials, err := a.Store.CredentialsForUser(ctx, u.ID)
		if err != nil || len(credentials) != 1 || credentials[0].Kind != protocols.Hysteria2 {
			t.Fatal("missing credentials", err)
		}
	}
	group := "Changed"
	if err = a.BulkEditUsers(ctx, []store.UserBatchSelection{{UserSelection: store.UserSelection{ID: users[0].ID, ExpectedEnabled: true}}}, store.UserBatchChanges{Group: &group}); err != nil {
		t.Fatal(err)
	}
	got, _, _ := a.Store.UserEditor(ctx, users[0].ID)
	if got.Preferences.Group != group || got.UUID != users[0].UUID {
		t.Fatal("metadata lost identity")
	}
}
