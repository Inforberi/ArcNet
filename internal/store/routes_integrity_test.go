package store

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/arcvpn/arcvpn/internal/routes"
)

func TestReferencedNodeCannotBeRevoked(t *testing.T) {
	for _, role := range []string{"entry", "primary", "backup"} {
		t.Run(role, func(t *testing.T) {
			ctx := context.Background()
			s, done := createControlTestStore(t)
			defer done()
			nodeID, _ := createControlTestNode(t, s)
			token, err := s.CreateEnrollment(ctx, "second")
			if err != nil {
				t.Fatal(err)
			}
			second, _, err := s.RegisterNode(ctx, token)
			if err != nil {
				t.Fatal(err)
			}
			p := routes.DefaultPolicy()
			p.Name = "Protected route"
			switch role {
			case "entry":
				p.EntryNode = nodeID
			case "primary":
				p.Mode = "cascade"
				p.PrimaryExit = nodeID
			case "backup":
				p.Mode = "cascade"
				p.PrimaryExit = second.ID
				p.BackupExits = []string{nodeID}
			}
			saved, err := s.SaveRoute(ctx, p, 0)
			if err != nil {
				t.Fatal(err)
			}
			var dependency *NodeRouteDependencyError
			if err = s.RemoveNode(ctx, nodeID); !errors.As(err, &dependency) || len(dependency.Names) != 1 || dependency.Names[0] != p.Name {
				t.Fatalf("dependency not reported: %v", err)
			}
			if err = s.DeleteRoute(ctx, saved.ID, saved.Version); err != nil {
				t.Fatal(err)
			}
			if err = s.RemoveNode(ctx, nodeID); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSSHRemovalAndRouteCreationExcludeEachOther(t *testing.T) {
	ctx := context.Background()
	s, done := createControlTestStore(t)
	defer done()
	nodeID, _ := createControlTestNode(t, s)
	original, err := s.CreateNodeBootstrap(ctx, "Remote", "9.9.9.9", 22, 443)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.AttachNodeBootstrap(ctx, original.ID, nodeID); err != nil {
		t.Fatal(err)
	}
	if err = s.UpdateNodeBootstrap(ctx, original.ID, "finished", "installed", ""); err != nil {
		t.Fatal(err)
	}
	p := routes.DefaultPolicy()
	p.Name = "Route during removal"
	p.Mode = "cascade"
	p.PrimaryExit = nodeID
	var saved routes.Policy
	var saveErr, removeErr error
	var wg sync.WaitGroup
	start := make(chan struct{})
	wg.Add(2)
	go func() { defer wg.Done(); <-start; saved, saveErr = s.SaveRoute(ctx, p, 0) }()
	go func() { defer wg.Done(); <-start; _, removeErr = s.CreateNodeRemoval(ctx, nodeID, "purge") }()
	close(start)
	wg.Wait()
	if (saveErr == nil) == (removeErr == nil) {
		t.Fatalf("expected one winner: save=%v remove=%v", saveErr, removeErr)
	}
	if saveErr == nil {
		if !errors.Is(removeErr, ErrNodeInUse) {
			t.Fatal(removeErr)
		}
		if err = s.DeleteRoute(ctx, saved.ID, saved.Version); err != nil {
			t.Fatal(err)
		}
		if _, err = s.CreateNodeRemoval(ctx, nodeID, "detach"); err != nil {
			t.Fatal(err)
		}
	} else if !errors.Is(saveErr, ErrRouteInvalid) {
		t.Fatal(saveErr)
	}
	if _, err = s.SaveRoute(ctx, p, 0); !errors.Is(err, ErrRouteInvalid) {
		t.Fatalf("removing node accepted by route: %v", err)
	}
}

func TestRestoreRoutesAtomicLegacyAndCAS(t *testing.T) {
	ctx := context.Background()
	s, done := createControlTestStore(t)
	defer done()
	nodeID, _ := createControlTestNode(t, s)
	p := routes.DefaultPolicy()
	p.Name = "Original"
	p.Mode = "cascade"
	p.PrimaryExit = nodeID
	saved, err := s.SaveRoute(ctx, p, 0)
	if err != nil {
		t.Fatal(err)
	}
	nodes, _, err := s.RouteBackupState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	emptyNodes := []NodeIdentity{}
	budget := 99
	if err = s.ReplaceFullStateWithManagement(ctx, nil, nil, nil, nil, &emptyNodes, &budget); !errors.Is(err, ErrRouteInvalid) {
		t.Fatalf("legacy import orphaned route: %v", err)
	}
	afterNodes, afterRoutes, err := s.RouteBackupState(ctx)
	if err != nil || len(afterNodes) != 1 || len(afterRoutes) != 1 || afterRoutes[0].Version != saved.Version {
		t.Fatalf("failed legacy import changed state: %v", err)
	}
	if got, _ := s.LogBudgetMB(ctx); got == budget {
		t.Fatal("failed restore changed budget")
	}
	restored := saved
	restored.Name = "Restored"
	imported := []routes.Policy{restored}
	if err = s.ReplaceFullStateWithRoutes(ctx, nil, nil, nil, nil, &nodes, &budget, &imported); err != nil {
		t.Fatal(err)
	}
	after, err := s.RoutePolicy(ctx, saved.ID)
	if err != nil || after.Name != "Restored" || after.Version <= saved.Version {
		t.Fatalf("restoration failed: %+v %v", after, err)
	}
	if _, err = s.SaveRoute(ctx, saved, saved.Version); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale form overwrote restore: %v", err)
	}
	bad := []routes.Policy{after}
	bad[0].PrimaryExit = "missing"
	if err = s.ReplaceFullStateWithRoutes(ctx, nil, nil, nil, nil, &emptyNodes, nil, &bad); !errors.Is(err, ErrRouteInvalid) {
		t.Fatal(err)
	}
	if got, err := s.RoutePolicy(ctx, saved.ID); err != nil || got.Name != after.Name || got.Version != after.Version {
		t.Fatalf("invalid import changed drafts: %+v %v", got, err)
	}
	emptyRoutes := []routes.Policy{}
	if err = s.ReplaceFullStateWithRoutes(ctx, nil, nil, nil, nil, &emptyNodes, nil, &emptyRoutes); err != nil {
		t.Fatal(err)
	}
	if got, err := s.RoutePolicies(ctx); err != nil || len(got) != 0 {
		t.Fatalf("explicit empty v4 routes not restored: %v", err)
	}
}

func TestRouteAPIRepresentationAlwaysHasBackupArray(t *testing.T) {
	ctx := context.Background()
	s, done := createControlTestStore(t)
	defer done()
	p := routes.DefaultPolicy()
	p.Name = "No reserves"
	p.BackupExits = nil
	saved, err := s.SaveRoute(ctx, p, 0)
	if err != nil || saved.BackupExits == nil {
		t.Fatalf("save returned null backups: %+v %v", saved, err)
	}
	read, err := s.RoutePolicy(ctx, saved.ID)
	if err != nil || read.BackupExits == nil {
		t.Fatalf("read returned null backups: %+v %v", read, err)
	}
}
