package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/arcvpn/arcvpn/internal/auth"
	"github.com/arcvpn/arcvpn/internal/db"
)

const cascadeSchema = `CREATE TABLE IF NOT EXISTS cascade_deployments (
 id TEXT PRIMARY KEY, route_id TEXT NOT NULL REFERENCES route_policies(id), route_version INTEGER NOT NULL,
 entry_node TEXT NOT NULL REFERENCES managed_nodes(id), exit_node TEXT NOT NULL REFERENCES managed_nodes(id),
 state TEXT NOT NULL, version INTEGER NOT NULL, secret_ciphertext TEXT NOT NULL,
 roster_digest TEXT NOT NULL, created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL)`
const cascadeStepsSchema = `CREATE TABLE IF NOT EXISTS cascade_steps (
 deployment_id TEXT NOT NULL REFERENCES cascade_deployments(id) ON DELETE CASCADE,
 step TEXT NOT NULL, node_id TEXT NOT NULL REFERENCES managed_nodes(id),
 kind TEXT NOT NULL, config_ciphertext TEXT NOT NULL, digest TEXT NOT NULL,
 operation_id TEXT NOT NULL DEFAULT '', revision INTEGER NOT NULL DEFAULT 0,
 PRIMARY KEY(deployment_id,step))`
const cascadeLeasesSchema = `CREATE TABLE IF NOT EXISTS cascade_leases (
 node_id TEXT PRIMARY KEY REFERENCES managed_nodes(id),
 deployment_id TEXT NOT NULL REFERENCES cascade_deployments(id) ON DELETE CASCADE)`
const cascadeCapabilitySchema = `CREATE TABLE IF NOT EXISTS cascade_capabilities (
 node_id TEXT PRIMARY KEY REFERENCES managed_nodes(id) ON DELETE CASCADE,
 protocol INTEGER NOT NULL, checked_at INTEGER NOT NULL)`

var ErrCascadeBusy = errors.New("node or route is reserved by a cascade deployment")
var ErrCascadeNotReady = errors.New("cascade requires fresh helper capabilities and acknowledged node baselines")

// CascadeDeployment is a public projection: never machine credentials/configs.
type CascadeDeployment struct {
	ID           string `json:"id"`
	RouteID      string `json:"route_id"`
	RouteVersion int64  `json:"route_version"`
	EntryNode    string `json:"entry_node"`
	ExitNode     string `json:"exit_node"`
	State        string `json:"state"`
	Version      int64  `json:"version"`
}
type CascadePreparation struct {
	RouteID                                string
	ExpectedVersion                        int64
	RosterDigest                           string
	EntryConfig, ExitConfig, ChannelSecret []byte
}

func cascadeProjection(r db.Row) CascadeDeployment {
	version, _ := strconv.ParseInt(r["version"], 10, 64)
	routeVersion, _ := strconv.ParseInt(r["route_version"], 10, 64)
	return CascadeDeployment{ID: r["id"], RouteID: r["route_id"], RouteVersion: routeVersion, EntryNode: r["entry_node"], ExitNode: r["exit_node"], State: r["state"], Version: version}
}
func cascadeRow(tx *db.Tx, id string) (db.Row, error) {
	rows, e := tx.Query("SELECT * FROM cascade_deployments WHERE id=?", id)
	if e != nil {
		return nil, e
	}
	if len(rows) != 1 {
		return nil, ErrNotFound
	}
	return rows[0], nil
}
func (s *Store) CascadeDeployment(ctx context.Context, id string) (CascadeDeployment, error) {
	var out CascadeDeployment
	e := s.DB.Read(ctx, func(tx *db.Tx) error {
		r, e := cascadeRow(tx, id)
		if e == nil {
			out = cascadeProjection(r)
		}
		return e
	})
	return out, e
}
func requireNodeWithoutCascade(tx *db.Tx, id string) error {
	rows, e := tx.Query("SELECT node_id FROM cascade_leases WHERE node_id=?", id)
	if e != nil {
		return e
	}
	if len(rows) > 0 {
		return ErrCascadeBusy
	}
	return nil
}
func requireRouteWithoutCascade(tx *db.Tx, id string) error {
	rows, e := tx.Query("SELECT id FROM cascade_deployments WHERE route_id=? LIMIT 1", id)
	if e != nil {
		return e
	}
	if len(rows) > 0 {
		return ErrCascadeBusy
	}
	return nil
}
func requireNoCascades(tx *db.Tx) error {
	rows, e := tx.Query("SELECT id FROM cascade_deployments LIMIT 1")
	if e != nil {
		return e
	}
	if len(rows) > 0 {
		return ErrCascadeBusy
	}
	return nil
}

// Roster changes invalidate preparation. Devices/finite packages are unsupported
// remotely and rejected rather than silently falling back to old shared keys.
func cascadeRoster(tx *db.Tx) (string, error) {
	devices, e := tx.Query("SELECT user_id FROM user_devices LIMIT 1")
	if e != nil {
		return "", e
	}
	if len(devices) > 0 {
		return "", ErrUsageUnsupported
	}
	packages, e := readPackages(tx)
	if e != nil {
		return "", e
	}
	for _, p := range packages {
		if p.QuotaBytes > 0 || p.ExpiresAt > 0 {
			return "", ErrUsageUnsupported
		}
	}
	rows, e := tx.Query("SELECT id,uuid,enabled FROM users ORDER BY id")
	if e != nil {
		return "", e
	}
	blob, _ := json.Marshal(rows)
	sum := sha256.Sum256(blob)
	return hex.EncodeToString(sum[:]), nil
}
func readyCascadeNode(tx *db.Tx, id string) (db.Row, error) {
	rows, e := tx.Query(`SELECT c.*,n.last_seen,n.agent_protocol,COALESCE(k.protocol,0) AS cascade_protocol,COALESCE(k.checked_at,0) AS capability_at FROM node_control c JOIN managed_nodes n ON n.id=c.node_id LEFT JOIN cascade_capabilities k ON k.node_id=c.node_id WHERE c.node_id=?`, id)
	if e != nil {
		return nil, e
	}
	if len(rows) != 1 {
		return nil, ErrNotFound
	}
	r := rows[0]
	seen, _ := strconv.ParseInt(r["last_seen"], 10, 64)
	checked, _ := strconv.ParseInt(r["capability_at"], 10, 64)
	if r["agent_protocol"] != "1" || r["cascade_protocol"] != "1" || time.Now().Unix()-seen > 180 || time.Now().Unix()-checked > 180 || r["draining"] != "0" || r["deployment_status"] != "applied" || r["desired_revision"] == "0" || r["desired_revision"] != r["applied_revision"] || r["desired_revision"] != r["last_good_revision"] {
		return nil, ErrCascadeNotReady
	}
	return r, nil
}
func (s *Store) PrepareCascade(ctx context.Context, input CascadePreparation) (CascadeDeployment, error) {
	if input.ExpectedVersion < 1 || len(input.ChannelSecret) < 1 || len(input.ChannelSecret) > 4096 || !json.Valid(input.ChannelSecret) {
		return CascadeDeployment{}, ErrRouteInvalid
	}
	for _, config := range [][]byte{input.EntryConfig, input.ExitConfig} {
		if len(config) == 0 || len(config) > 252<<10 || !json.Valid(config) {
			return CascadeDeployment{}, ErrRouteInvalid
		}
	}
	id, e := auth.UUID()
	if e != nil {
		return CascadeDeployment{}, e
	}
	var out CascadeDeployment
	e = s.DB.WithTx(ctx, func(tx *db.Tx) error {
		count, e := tx.Query("SELECT id FROM cascade_deployments LIMIT 32")
		if e != nil {
			return e
		}
		if len(count) >= 32 {
			return ErrCascadeBusy
		}
		rows, e := tx.Query("SELECT body,version FROM route_policies WHERE id=?", input.RouteID)
		if e != nil {
			return e
		}
		if len(rows) != 1 {
			return ErrNotFound
		}
		if rows[0]["version"] != strconv.FormatInt(input.ExpectedVersion, 10) {
			return ErrConflict
		}
		var policy struct {
			Mode    string   `json:"mode"`
			Entry   string   `json:"entry_node"`
			Exit    string   `json:"primary_exit"`
			Backups []string `json:"backup_exits"`
			Failure string   `json:"failure_mode"`
		}
		if json.Unmarshal([]byte(rows[0]["body"]), &policy) != nil || policy.Mode != "cascade" || policy.Failure != "closed" || policy.Entry == "local" || policy.Exit == "local" || len(policy.Backups) != 0 {
			return ErrRouteInvalid
		}
		if e := requireRouteWithoutCascade(tx, input.RouteID); e != nil {
			return e
		}
		roster, e := cascadeRoster(tx)
		if e != nil {
			return e
		}
		if roster != input.RosterDigest {
			return ErrConflict
		}
		baselines := map[string]db.Row{}
		for _, node := range []string{policy.Entry, policy.Exit} {
			if e := requireNodeWithoutCascade(tx, node); e != nil {
				return e
			}
			r, e := readyCascadeNode(tx, node)
			if e != nil {
				return e
			}
			baselines[node] = r
		}
		sealed, e := s.sealNodeConfig(input.ChannelSecret, "cascade-secret:"+id)
		if e != nil {
			return e
		}
		now := strconv.FormatInt(time.Now().Unix(), 10)
		if e := tx.Exec("INSERT INTO cascade_deployments(id,route_id,route_version,entry_node,exit_node,state,version,secret_ciphertext,roster_digest,created_at,updated_at) VALUES(?,?,?,?,?,'prepared',1,?,?,?,?)", id, input.RouteID, strconv.FormatInt(input.ExpectedVersion, 10), policy.Entry, policy.Exit, sealed, roster, now, now); e != nil {
			return e
		}
		for _, node := range []string{policy.Entry, policy.Exit} {
			if e := tx.Exec("INSERT INTO cascade_leases(node_id,deployment_id) VALUES(?,?)", node, id); e != nil {
				return ErrCascadeBusy
			}
		}
		for step, config := range map[string][]byte{"apply_entry": input.EntryConfig, "apply_exit": input.ExitConfig} {
			node := policy.Entry
			if step == "apply_exit" {
				node = policy.Exit
			}
			if e := s.saveCascadeStep(tx, id, step, node, "apply_cascade_xray_v1", config); e != nil {
				return e
			}
		}
		for step, node := range map[string]string{"restore_entry": policy.Entry, "restore_exit": policy.Exit} {
			r := baselines[node]
			plain, e := s.openNodeConfig(r["desired_ciphertext"], fmt.Sprintf("%s:%s:%s", node, r["desired_revision"], r["desired_digest"]))
			if e != nil {
				return e
			}
			ops, e := tx.Query("SELECT kind FROM node_operations WHERE id=?", r["deployment_id"])
			if e != nil {
				return e
			}
			if len(ops) != 1 || ops[0]["kind"] != "apply_xray_v1" {
				return ErrCascadeNotReady
			}
			if e := s.saveCascadeStep(tx, id, step, node, "apply_xray_v1", plain); e != nil {
				return e
			}
		}
		row, e := cascadeRow(tx, id)
		if e == nil {
			out = cascadeProjection(row)
		}
		return e
	})
	return out, e
}
func (s *Store) saveCascadeStep(tx *db.Tx, id, step, node, kind string, config []byte) error {
	hash := sha256.Sum256(config)
	digest := hex.EncodeToString(hash[:])
	sealed, e := s.sealNodeConfig(config, "cascade-step:"+id+":"+step+":"+digest)
	if e != nil {
		return e
	}
	return tx.Exec("INSERT INTO cascade_steps(deployment_id,step,node_id,kind,config_ciphertext,digest) VALUES(?,?,?,?,?,?)", id, step, node, kind, sealed, digest)
}
func (s *Store) queueCascadeStep(tx *db.Tx, id, step, state string) error {
	rows, e := tx.Query("SELECT * FROM cascade_steps WHERE deployment_id=? AND step=?", id, step)
	if e != nil {
		return e
	}
	if len(rows) != 1 {
		return ErrNotFound
	}
	r := rows[0]
	plain, e := s.openNodeConfig(r["config_ciphertext"], "cascade-step:"+id+":"+step+":"+r["digest"])
	if e != nil {
		return e
	}
	// Replacements always receive a newer node revision, including compensation.
	op, e := s.queueNodeDeployment(tx, r["node_id"], plain, r["kind"])
	if e != nil {
		return e
	}
	if e := tx.Exec("UPDATE cascade_steps SET operation_id=?,revision=? WHERE deployment_id=? AND step=?", op.ID, strconv.FormatInt(op.Revision, 10), id, step); e != nil {
		return e
	}
	return tx.Exec("UPDATE cascade_deployments SET state=?,version=version+1,updated_at=? WHERE id=?", state, strconv.FormatInt(time.Now().Unix(), 10), id)
}
func (s *Store) StartCascade(ctx context.Context, id string, expected int64) (CascadeDeployment, error) {
	var out CascadeDeployment
	e := s.DB.WithTx(ctx, func(tx *db.Tx) error {
		r, e := cascadeRow(tx, id)
		if e != nil {
			return e
		}
		if r["state"] != "prepared" || r["version"] != strconv.FormatInt(expected, 10) {
			return ErrConflict
		}
		roster, e := cascadeRoster(tx)
		if e != nil {
			return e
		}
		if roster != r["roster_digest"] {
			return ErrConflict
		}
		for _, node := range []string{r["entry_node"], r["exit_node"]} {
			if _, e := readyCascadeNode(tx, node); e != nil {
				return e
			}
		}
		if e := tx.Exec("UPDATE node_control SET published=0,probe='unknown',probe_revision=0,probe_checked_at=0 WHERE node_id IN (?,?)", r["entry_node"], r["exit_node"]); e != nil {
			return e
		}
		if e := s.queueCascadeStep(tx, id, "apply_exit", "exit_queued"); e != nil {
			return e
		}
		r, e = cascadeRow(tx, id)
		if e == nil {
			out = cascadeProjection(r)
		}
		return e
	})
	return out, e
}

// Called transactionally with the authenticated per-node ACK, never heartbeat.
func (s *Store) advanceCascadeACK(tx *db.Tx, operation, result string) error {
	rows, e := tx.Query("SELECT d.*,s.step FROM cascade_steps s JOIN cascade_deployments d ON d.id=s.deployment_id WHERE s.operation_id=?", operation)
	if e != nil || len(rows) == 0 {
		return e
	}
	r := rows[0]
	step, state := r["step"], r["state"]
	expected := map[string]string{"apply_exit": "exit_queued", "apply_entry": "entry_queued", "restore_entry": "rollback_entry", "restore_exit": "rollback_exit"}
	if state != expected[step] {
		return nil
	}
	if result != "applied" {
		if step == "restore_entry" || step == "restore_exit" {
			return markCascadeAttention(tx, r["id"])
		}
		if step == "apply_entry" {
			return s.queueCascadeStep(tx, r["id"], "restore_entry", "rollback_entry")
		}
		return s.queueCascadeStep(tx, r["id"], "restore_exit", "rollback_exit")
	}
	switch step {
	case "apply_exit":
		roster, e := cascadeRoster(tx)
		if e != nil || roster != r["roster_digest"] {
			return s.queueCascadeStep(tx, r["id"], "restore_exit", "rollback_exit")
		}
		return s.queueCascadeStep(tx, r["id"], "apply_entry", "entry_queued")
	case "apply_entry":
		return tx.Exec("UPDATE cascade_deployments SET state='awaiting_probe',version=version+1,updated_at=? WHERE id=?", strconv.FormatInt(time.Now().Unix(), 10), r["id"])
	case "restore_entry":
		return s.queueCascadeStep(tx, r["id"], "restore_exit", "rollback_exit")
	case "restore_exit":
		if e := tx.Exec("UPDATE cascade_deployments SET state='rolled_back',version=version+1,secret_ciphertext='',updated_at=? WHERE id=?", strconv.FormatInt(time.Now().Unix(), 10), r["id"]); e != nil {
			return e
		}
		return tx.Exec("DELETE FROM cascade_leases WHERE deployment_id=?", r["id"])
	}
	return ErrConflict
}
func markCascadeAttention(tx *db.Tx, id string) error {
	return tx.Exec("UPDATE cascade_deployments SET state='attention',version=version+1,updated_at=? WHERE id=?", strconv.FormatInt(time.Now().Unix(), 10), id)
}
func markCascadeOperationUnknown(tx *db.Tx, operation string) error {
	rows, e := tx.Query("SELECT deployment_id FROM cascade_steps WHERE operation_id=?", operation)
	if e != nil {
		return e
	}
	for _, r := range rows {
		if e := markCascadeAttention(tx, r["deployment_id"]); e != nil {
			return e
		}
	}
	return nil
}

// Abort/unknown ACK recovery restores entry BEFORE removing the exit channel.
// It never assumes that an unacknowledged operation was not applied.
func (s *Store) RollbackCascade(ctx context.Context, id string, expected int64) error {
	return s.DB.WithTx(ctx, func(tx *db.Tx) error {
		r, e := cascadeRow(tx, id)
		if e != nil {
			return e
		}
		if r["version"] != strconv.FormatInt(expected, 10) || r["state"] == "rolled_back" || r["state"] == "prepared" {
			return ErrConflict
		}
		for _, node := range []string{r["entry_node"], r["exit_node"]} {
			if e := tx.Exec("UPDATE node_operations SET state='cancelled' WHERE id IN (SELECT deployment_id FROM node_control WHERE node_id=?) AND state IN ('queued','running')", node); e != nil {
				return e
			}
			if e := tx.Exec("UPDATE node_control SET deployment_status='failed',published=0 WHERE node_id=?", node); e != nil {
				return e
			}
		}
		return s.queueCascadeStep(tx, id, "restore_entry", "rollback_entry")
	})
}

// Only never-started or fully compensated journals can be discarded. Runtime
// state is deliberately not restored from a portable VPN backup.
func (s *Store) DeleteCascade(ctx context.Context, id string, expected int64) error {
	return s.DB.WithTx(ctx, func(tx *db.Tx) error {
		r, e := cascadeRow(tx, id)
		if e != nil {
			return e
		}
		if r["version"] != strconv.FormatInt(expected, 10) || (r["state"] != "prepared" && r["state"] != "rolled_back") {
			return ErrConflict
		}
		return tx.Exec("DELETE FROM cascade_deployments WHERE id=?", id)
	})
}

func requireNoCascadeLeases(tx *db.Tx) error {
	rows, e := tx.Query("SELECT node_id FROM cascade_leases LIMIT 1")
	if e != nil {
		return e
	}
	if len(rows) > 0 {
		return ErrCascadeBusy
	}
	return nil
}
func (s *Store) CascadeRoster(ctx context.Context) (string, error) {
	var digest string
	e := s.DB.Read(ctx, func(tx *db.Tx) error { var e error; digest, e = cascadeRoster(tx); return e })
	return digest, e
}
