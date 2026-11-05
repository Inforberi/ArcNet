package store

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/arcvpn/arcvpn/internal/db"
)

func TestUserCountsMatchesEnabledConfiguration(t *testing.T) {
	state, done := createControlTestStore(t)
	defer done()
	ctx := context.Background()
	for i, enabled := range []bool{true, false, true} {
		if err := state.InsertUser(ctx, User{Name: fmt.Sprintf("user-%d", i), UUID: fmt.Sprintf("uuid-%d", i), Subscription: fmt.Sprintf("token-%d", i), Enabled: enabled}); err != nil {
			t.Fatal(err)
		}
	}
	total, enabled, err := state.UserCounts(ctx)
	if err != nil || total != 3 || enabled != 2 {
		t.Fatalf("counts: %d %d %v", total, enabled, err)
	}
}
func BenchmarkOverviewUserCounts(b *testing.B) {
	database, err := db.Open(filepath.Join(b.TempDir(), "panel.sqlite"))
	if err != nil {
		b.Fatal(err)
	}
	defer database.Close()
	state := &Store{DB: database}
	ctx := context.Background()
	if err = state.Migrate(ctx); err != nil {
		b.Fatal(err)
	}
	err = database.WithTx(ctx, func(tx *db.Tx) error {
		for i := 0; i < 10000; i++ {
			if err := tx.Exec("INSERT INTO users(name,uuid,token,enabled,created_at) VALUES(?,?,?,?,0)", fmt.Sprintf("bench-%d", i), fmt.Sprintf("uuid-%d", i), fmt.Sprintf("token-%d", i), fmt.Sprint(i%2)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		b.Fatal(err)
	}
	b.Run("aggregate", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, _, err := state.UserCounts(ctx); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("previous-full-list", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, err := state.ListUsers(ctx); err != nil {
				b.Fatal(err)
			}
		}
	})
}
