package store

import (
	"context"
	"errors"
	"github.com/arcvpn/arcvpn/internal/db"
	"github.com/arcvpn/arcvpn/internal/routes"
	"path/filepath"
	"testing"
)

func TestRoutePolicyStoreMigrationConcurrencyAndGraph(t *testing.T) {
	ctx := context.Background()
	database, e := db.Open(filepath.Join(t.TempDir(), "state.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer database.Close()
	st := &Store{DB: database}
	if e = st.Migrate(ctx); e != nil {
		t.Fatal(e)
	}
	if e = st.Migrate(ctx); e != nil {
		t.Fatal("migration is not repeatable", e)
	}
	first := routes.DefaultPolicy()
	first.Name = "direct local"
	saved, e := st.SaveRoute(ctx, first, 0)
	if e != nil {
		t.Fatal(e)
	}
	if saved.ID == "" || saved.Version != 1 {
		t.Fatal(saved)
	}
	read, e := st.RoutePolicy(ctx, saved.ID)
	if e != nil || read.Mode != "direct" {
		t.Fatal(read, e)
	}
	if _, e = st.SaveRoute(ctx, saved, 44); !errors.Is(e, ErrConflict) {
		t.Fatal("stale update allowed", e)
	}
	read.Name = "direct revised"
	updated, e := st.SaveRoute(ctx, read, read.Version)
	if e != nil || updated.Version != 2 {
		t.Fatal(updated, e)
	}
	if e = st.DeleteRoute(ctx, saved.ID, 1); !errors.Is(e, ErrConflict) {
		t.Fatal("stale deletion allowed", e)
	}
	if e = st.DeleteRoute(ctx, saved.ID, 2); e != nil {
		t.Fatal(e)
	}
	if list, e := st.RoutePolicies(ctx); e != nil || len(list) != 0 {
		t.Fatal(list, e)
	}
	p := routes.DefaultPolicy()
	p.Name = "invalid node"
	p.EntryNode = "deleted-node"
	if _, e = st.SaveRoute(ctx, p, 0); !errors.Is(e, ErrRouteInvalid) {
		t.Fatal("phantom node allowed", e)
	}
}
func TestRouteGraphValidationWithExistingNodes(t *testing.T) {
	ctx := context.Background()
	d, e := db.Open(filepath.Join(t.TempDir(), "state.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer d.Close()
	s := &Store{DB: d}
	if e = s.Migrate(ctx); e != nil {
		t.Fatal(e)
	}
	for _, id := range []string{"a", "b", "c"} {
		if e = d.WithTx(ctx, func(tx *db.Tx) error {
			return tx.Exec("INSERT INTO managed_nodes(id,name,token_hash,created_at) VALUES(?,?,?,?)", id, id, "hash"+id, "1")
		}); e != nil {
			t.Fatal(e)
		}
	}
	mk := func(name, entry, exit string) routes.Policy {
		p := routes.DefaultPolicy()
		p.Mode = "cascade"
		p.Name = name
		p.EntryNode = entry
		p.PrimaryExit = exit
		return p
	}
	first, e := s.SaveRoute(ctx, mk("A to B", "a", "b"), 0)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.SaveRoute(ctx, mk("B to A", "b", "a"), 0); !errors.Is(e, ErrRouteInvalid) {
		t.Fatal("cycle allowed", e)
	}
	if _, _, e = s.PreviewRoute(ctx, mk("C to A", "c", "a")); e != nil {
		t.Fatal(e)
	}
	if _, _, e = s.PreviewRoute(ctx, mk("B to A", "b", "a")); !errors.Is(e, ErrRouteInvalid) {
		t.Fatal("invalid preview allowed", e)
	}
	got, e := s.RoutePolicies(ctx)
	if e != nil || len(got) != 1 || got[0].ID != first.ID {
		t.Fatal(got, e)
	}
}
