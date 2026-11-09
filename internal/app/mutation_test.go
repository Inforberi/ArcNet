package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestUserMutationFailedRecoveryIsNeverReportedAsRestored(t *testing.T) {
	for _, action := range []string{"create", "toggle", "delete"} {
		t.Run(action, func(t *testing.T) {
			a := firstProtocolApp(t)
			ctx := context.Background()
			if err := a.Setup(ctx, "198.51.100.25", "www.example.com", 443); err != nil {
				t.Fatal(err)
			}
			user, err := a.AddUser(ctx, "Keep me")
			if err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(filepath.Join(filepath.Dir(a.Core.Binary), "fail"), []byte("1"), 0600); err != nil {
				t.Fatal(err)
			}
			switch action {
			case "create":
				_, err = a.AddUser(ctx, "Must not remain")
			case "toggle":
				err = a.ToggleUser(ctx, user.ID, false)
			case "delete":
				err = a.DeleteUser(ctx, user.ID)
			}
			var failure *MutationError
			if !errors.As(err, &failure) || failure.Recovered {
				t.Fatalf("recovery falsely confirmed: %v", err)
			}
			got, readErr := a.Store.ByID(ctx, user.ID)
			if readErr != nil || got.UUID != user.UUID || !got.Enabled {
				t.Fatal("existing user lost", readErr)
			}
			users, readErr := a.Store.ListUsers(ctx)
			if readErr != nil || len(users) != 1 {
				t.Fatal("create rollback failed", readErr)
			}
		})
	}
}
