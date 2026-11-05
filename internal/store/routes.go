package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/arcvpn/arcvpn/internal/auth"
	"github.com/arcvpn/arcvpn/internal/db"
	"github.com/arcvpn/arcvpn/internal/routes"
)

// Route policies are deliberately drafts. A saved row has no data-plane effect.
// Later stages must introduce a separately audited publish/apply state machine.
const routePolicySchema = `CREATE TABLE IF NOT EXISTS route_policies (
 id TEXT PRIMARY KEY, body TEXT NOT NULL,
 version INTEGER NOT NULL CHECK(version > 0), updated_at INTEGER NOT NULL)`

var ErrRouteInvalid = errors.New("invalid routing policy")

var ErrNodeInUse = errors.New("node is referenced by routing drafts")

type NodeRouteDependencyError struct {
	Names []string
}

func (e *NodeRouteDependencyError) Error() string {
	return fmt.Sprintf("%v: %v", ErrNodeInUse, e.Names)
}
func (e *NodeRouteDependencyError) Unwrap() error { return ErrNodeInUse }

// Called in the same transaction as revocation/removal, so a concurrent route
// cannot acquire a reference between checking dependencies and removing a node.
func requireNodeWithoutRoutes(tx *db.Tx, id string) error {
	policies, err := readRoutes(tx)
	if err != nil {
		return err
	}
	names := []string{}
	for _, p := range policies {
		used := p.EntryNode == id || p.PrimaryExit == id
		for _, backup := range p.BackupExits {
			used = used || backup == id
		}
		if used {
			names = append(names, p.Name)
		}
	}
	if len(names) > 0 {
		return &NodeRouteDependencyError{Names: names}
	}
	return nil
}

func replaceRouteDrafts(tx *db.Tx, policies, previous []routes.Policy) error {
	if err := checkRoutes(tx, policies); err != nil {
		return err
	}
	versions := make(map[string]int64, len(previous))
	for _, p := range previous {
		versions[p.ID] = p.Version
	}
	if err := tx.Exec("DELETE FROM route_policies"); err != nil {
		return err
	}
	for _, p := range policies {
		// Advance CAS even when restoring an older copy with the same ID.
		// A form opened before restore must never overwrite restored content.
		if versions[p.ID] > p.Version {
			p.Version = versions[p.ID]
		}
		if p.Version < 1 || p.Version == math.MaxInt64 {
			return ErrRouteInvalid
		}
		p.Version++
		p.UpdatedAt = time.Now().Unix()
		if p.BackupExits == nil {
			p.BackupExits = []string{}
		}
		body, err := json.Marshal(p)
		if err != nil {
			return err
		}
		if err = tx.Exec("INSERT INTO route_policies(id,body,version,updated_at) VALUES(?,?,?,?)", p.ID, string(body), strconv.FormatInt(p.Version, 10), strconv.FormatInt(p.UpdatedAt, 10)); err != nil {
			return err
		}
	}
	return nil
}

func parseRoutes(rows []db.Row) ([]routes.Policy, error) {
	result := make([]routes.Policy, 0, len(rows))
	for _, row := range rows {
		var p routes.Policy
		if err := json.Unmarshal([]byte(row["body"]), &p); err != nil {
			return nil, fmt.Errorf("corrupt route %q: %w", row["id"], err)
		}
		v, err := strconv.ParseInt(row["version"], 10, 64)
		if err != nil || v < 1 || p.ID != row["id"] {
			return nil, fmt.Errorf("invalid route revision for %q", row["id"])
		}
		updated, err := strconv.ParseInt(row["updated_at"], 10, 64)
		if err != nil {
			return nil, err
		}
		if p.BackupExits == nil {
			p.BackupExits = []string{}
		}
		p.Version = v
		p.UpdatedAt = updated
		result = append(result, p)
	}
	return result, nil
}
func readRoutes(tx *db.Tx) ([]routes.Policy, error) {
	rows, err := tx.Query("SELECT id,body,version,updated_at FROM route_policies ORDER BY id")
	if err != nil {
		return nil, err
	}
	return parseRoutes(rows)
}
func (s *Store) RoutePolicies(ctx context.Context) ([]routes.Policy, error) {
	result := []routes.Policy{}
	err := s.DB.Read(ctx, func(tx *db.Tx) error { var e error; result, e = readRoutes(tx); return e })
	return result, err
}
func (s *Store) RoutePolicy(ctx context.Context, id string) (routes.Policy, error) {
	var result routes.Policy
	err := s.DB.Read(ctx, func(tx *db.Tx) error {
		rows, e := tx.Query("SELECT id,body,version,updated_at FROM route_policies WHERE id=?", id)
		if e != nil {
			return e
		}
		if len(rows) == 0 {
			return ErrNotFound
		}
		ps, e := parseRoutes(rows)
		if e != nil {
			return e
		}
		result = ps[0]
		return nil
	})
	return result, err
}

// CheckRoutes validates the entire draft graph atomically against known node IDs.
func checkRoutes(tx *db.Tx, policies []routes.Policy) error {
	if e := routes.ValidateGraph(policies); e != nil {
		return fmt.Errorf("%w: %v", ErrRouteInvalid, e)
	}
	nodes, e := tx.Query("SELECT id FROM managed_nodes")
	if e != nil {
		return e
	}
	known := map[string]bool{routes.LocalNode: true}
	for _, node := range nodes {
		known[node["id"]] = true
	}
	removals, e := tx.Query("SELECT node_id FROM node_bootstrap_operations WHERE operation_type='remove' AND status='running'")
	if e != nil {
		return e
	}
	for _, removal := range removals {
		delete(known, removal["node_id"])
	}
	for _, p := range policies {
		refs := []string{p.EntryNode}
		if p.Mode == "cascade" {
			refs = append(refs, p.PrimaryExit)
			refs = append(refs, p.BackupExits...)
		}
		for _, id := range refs {
			if !known[id] {
				return fmt.Errorf("%w: node %q does not exist", ErrRouteInvalid, id)
			}
		}
	}
	return nil
}

// PreviewRoute performs the same validation as SaveRoute and returns a preview
// without creating a row or allocating credentials on a remote node.
func (s *Store) PreviewRoute(ctx context.Context, p routes.Policy) ([]routes.Edge, []routes.Change, error) {
	var result []routes.Edge
	changes := []routes.Change{}
	err := s.DB.Read(ctx, func(tx *db.Tx) error {
		policies, e := readRoutes(tx)
		if e != nil {
			return e
		}
		if p.ID == "" {
			p.ID = "preview"
		}
		updated := false
		for i := range policies {
			if policies[i].ID == p.ID {
				changes = routes.Diff(&policies[i], p)
				policies[i] = p
				updated = true
				break
			}
		}
		if !updated {
			changes = routes.Diff(nil, p)
			policies = append(policies, p)
		}
		if e = checkRoutes(tx, policies); e != nil {
			return e
		}
		result = routes.Edges(policies)
		return nil
	})
	return result, changes, err
}

// SaveRoute enforces optimistic versioning. expectedVersion=0 creates;
// nonzero updates, and any stale writer receives ErrConflict.
func (s *Store) SaveRoute(ctx context.Context, p routes.Policy, expectedVersion int64) (routes.Policy, error) {
	if expectedVersion < 0 {
		return routes.Policy{}, ErrRouteInvalid
	}
	if expectedVersion == 0 {
		id, e := auth.UUID()
		if e != nil {
			return routes.Policy{}, e
		}
		p.ID = id
	}
	// Caller may not write metadata: versions and timestamps are server-owned.
	p.Version = 0
	if p.BackupExits == nil {
		p.BackupExits = []string{}
	}
	p.UpdatedAt = 0
	var result routes.Policy
	err := s.DB.WithTx(ctx, func(tx *db.Tx) error {
		existing, e := readRoutes(tx)
		if e != nil {
			return e
		}
		index := -1
		for i := range existing {
			if existing[i].ID == p.ID {
				index = i
				break
			}
		}
		if expectedVersion == 0 && index >= 0 {
			return ErrConflict
		}
		if expectedVersion > 0 && (index < 0 || existing[index].Version != expectedVersion) {
			return ErrConflict
		}
		now := time.Now().Unix()
		p.Version = expectedVersion + 1
		p.UpdatedAt = now
		candidate := append([]routes.Policy(nil), existing...)
		if index >= 0 {
			candidate[index] = p
		} else {
			candidate = append(candidate, p)
		}
		if e = checkRoutes(tx, candidate); e != nil {
			return e
		}
		data, e := json.Marshal(p)
		if e != nil {
			return e
		}
		if index >= 0 {
			e = tx.Exec("UPDATE route_policies SET body=?,version=?,updated_at=? WHERE id=? AND version=?", string(data), strconv.FormatInt(p.Version, 10), strconv.FormatInt(now, 10), p.ID, strconv.FormatInt(expectedVersion, 10))
		} else {
			e = tx.Exec("INSERT INTO route_policies(id,body,version,updated_at) VALUES(?,?,?,?)", p.ID, string(data), "1", strconv.FormatInt(now, 10))
		}
		if e != nil {
			return e
		}
		result = p
		return nil
	})
	return result, err
}

func (s *Store) DeleteRoute(ctx context.Context, id string, expectedVersion int64) error {
	if expectedVersion < 1 {
		return ErrConflict
	}
	return s.DB.WithTx(ctx, func(tx *db.Tx) error {
		rows, e := tx.Query("SELECT version FROM route_policies WHERE id=?", id)
		if e != nil {
			return e
		}
		if len(rows) == 0 {
			return ErrNotFound
		}
		current, e := strconv.ParseInt(rows[0]["version"], 10, 64)
		if e != nil {
			return e
		}
		if current != expectedVersion {
			return ErrConflict
		}
		return tx.Exec("DELETE FROM route_policies WHERE id=? AND version=?", id, strconv.FormatInt(expectedVersion, 10))
	})
}
