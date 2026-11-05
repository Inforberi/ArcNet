package store

import (
	"context"
	"errors"
	"github.com/arcvpn/arcvpn/internal/auth"
	"github.com/arcvpn/arcvpn/internal/db"
	"strconv"
	"time"
)

const nodeBootstrapSchema = `CREATE TABLE IF NOT EXISTS node_bootstrap_operations (
 id TEXT PRIMARY KEY,
 name TEXT NOT NULL,
 address TEXT NOT NULL,
 ssh_port INTEGER NOT NULL,
 vpn_port INTEGER NOT NULL,
 status TEXT NOT NULL,
 step TEXT NOT NULL,
 created_at INTEGER NOT NULL,
 updated_at INTEGER NOT NULL,
 node_id TEXT NOT NULL DEFAULT '',
 error TEXT NOT NULL DEFAULT '',
 operation_type TEXT NOT NULL DEFAULT 'install',
 removal_mode TEXT NOT NULL DEFAULT '',
 target_version TEXT NOT NULL DEFAULT ''
)`

type NodeBootstrapOperation struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Address       string `json:"address"`
	SSHPort       int    `json:"ssh_port"`
	VPNPort       int    `json:"vpn_port"`
	Status        string `json:"status"`
	Step          string `json:"step"`
	CreatedAt     int64  `json:"created_at"`
	UpdatedAt     int64  `json:"updated_at"`
	NodeID        string `json:"node_id,omitempty"`
	Error         string `json:"error,omitempty"`
	OperationType string `json:"operation_type"`
	RemovalMode   string `json:"removal_mode,omitempty"`
	TargetVersion string `json:"target_version,omitempty"`
}

// SSH lifecycle events have a separate 50-item UI journal. Retain active
// installation provenance for as long as the managed identity exists; it is
// needed to authorize destructive remote operations. Other terminal entries
// are bounded by age and count, while in-flight operations are never pruned.
func pruneBootstrapHistory(tx *db.Tx, now int64) error {
	cutoff := strconv.FormatInt(now-int64((90*24*time.Hour)/time.Second), 10)
	return tx.Exec(`DELETE FROM node_bootstrap_operations
		WHERE status IN ('installed','failed','removed','detached','upgraded')
		AND NOT (operation_type='install' AND status='installed'
			AND node_id IN (SELECT id FROM managed_nodes))
		AND id NOT IN (
			SELECT id FROM node_bootstrap_operations
			ORDER BY created_at DESC, id DESC LIMIT 50
		)
		AND (updated_at < ? OR id NOT IN (
			SELECT id FROM node_bootstrap_operations
			ORDER BY created_at DESC, id DESC LIMIT 512
		))`, cutoff)
}

func (s *Store) CreateNodeBootstrap(ctx context.Context, name, address string, sshPort, vpnPort int) (NodeBootstrapOperation, error) {
	id, e := auth.UUID()
	if e != nil {
		return NodeBootstrapOperation{}, e
	}
	now := time.Now().Unix()
	op := NodeBootstrapOperation{ID: id, Name: name, Address: address, SSHPort: sshPort, VPNPort: vpnPort, Status: "running", Step: "queued", CreatedAt: now, UpdatedAt: now, OperationType: "install"}
	e = s.DB.WithTx(ctx, func(tx *db.Tx) error {
		if err := tx.Exec("INSERT INTO node_bootstrap_operations(id,name,address,ssh_port,vpn_port,status,step,created_at,updated_at,operation_type) VALUES(?,?,?,?,?,'running','queued',?,?,'install')", id, name, address, strconv.Itoa(sshPort), strconv.Itoa(vpnPort), strconv.FormatInt(now, 10), strconv.FormatInt(now, 10)); err != nil {
			return err
		}
		return pruneBootstrapHistory(tx, now)
	})
	return op, e
}
func (s *Store) UpdateNodeBootstrap(ctx context.Context, id, step, status, errorCode string) error {
	allowed := map[string]bool{"queued": true, "preflight": true, "verify_release": true, "stage": true, "install": true, "enroll": true, "start": true, "apply": true, "finished": true, "manual_apply": true, "inspect": true, "remove": true, "revoke": true, "upgrade": true}
	if !allowed[step] {
		return errors.New("invalid install phase")
	}
	if status != "running" && status != "failed" && status != "installed" && status != "removed" && status != "detached" && status != "upgraded" {
		return errors.New("invalid install state")
	}
	if len(errorCode) > 150 {
		return errors.New("installer error string exceeds limit")
	}
	return s.DB.WithTx(ctx, func(tx *db.Tx) error {
		return tx.Exec("UPDATE node_bootstrap_operations SET step=?,status=?,error=?,updated_at=? WHERE id=? AND status='running'", step, status, errorCode, strconv.FormatInt(time.Now().Unix(), 10), id)
	})
}

// AttachNodeBootstrap links the persistent journal to a preallocated node ID.
func (s *Store) AttachNodeBootstrap(ctx context.Context, id, nodeID string) error {
	if nodeID == "" {
		return errors.New("node ID missing")
	}
	return s.DB.WithTx(ctx, func(tx *db.Tx) error {
		return tx.Exec("UPDATE node_bootstrap_operations SET node_id=? WHERE id=? AND status='running'", nodeID, id)
	})
}
func (s *Store) NodeBootstrap(ctx context.Context, id string) (NodeBootstrapOperation, error) {
	var op NodeBootstrapOperation
	e := s.DB.Read(ctx, func(tx *db.Tx) error {
		rows, e := tx.Query("SELECT * FROM node_bootstrap_operations WHERE id=?", id)
		if e != nil {
			return e
		}
		if len(rows) == 0 {
			return ErrNotFound
		}
		r := rows[0]
		op = NodeBootstrapOperation{ID: id, Name: r["name"], Address: r["address"], Status: r["status"], Step: r["step"], NodeID: r["node_id"], Error: r["error"], OperationType: r["operation_type"], RemovalMode: r["removal_mode"], TargetVersion: r["target_version"]}
		op.SSHPort, _ = strconv.Atoi(r["ssh_port"])
		op.VPNPort, _ = strconv.Atoi(r["vpn_port"])
		op.CreatedAt, _ = strconv.ParseInt(r["created_at"], 10, 64)
		op.UpdatedAt, _ = strconv.ParseInt(r["updated_at"], 10, 64)
		return nil
	})
	return op, e
}
func (s *Store) ListNodeBootstraps(ctx context.Context) ([]NodeBootstrapOperation, error) {
	out := make([]NodeBootstrapOperation, 0)
	e := s.DB.Read(ctx, func(tx *db.Tx) error {
		rows, e := tx.Query("SELECT id FROM node_bootstrap_operations ORDER BY created_at DESC,id DESC LIMIT 50")
		if e != nil {
			return e
		}
		for _, r := range rows {
			out = append(out, NodeBootstrapOperation{ID: r["id"]})
		}
		return nil
	})
	if e != nil {
		return nil, e
	}
	for idx := range out {
		op, e := s.NodeBootstrap(ctx, out[idx].ID)
		if e != nil {
			return nil, e
		}
		out[idx] = op
	}
	return out, nil
}

// A process restart loses ephemeral SSH keys by design. Therefore unfinished
// operations are explicitly marked interrupted, never shown as succeeded.
func (s *Store) MarkInterruptedBootstraps(ctx context.Context) error {
	return s.DB.WithTx(ctx, func(tx *db.Tx) error {
		return tx.Exec("UPDATE node_bootstrap_operations SET status='failed',error='controller restart interrupted SSH operation',updated_at=? WHERE status='running'", strconv.FormatInt(time.Now().Unix(), 10))
	})
}

// InstalledBootstrapForNode is the immutable origin of the remote installation
// marker. Unregistered/manual nodes cannot be destructively changed over SSH.
func (s *Store) InstalledBootstrapForNode(ctx context.Context, nodeID string) (NodeBootstrapOperation, error) {
	var id string
	err := s.DB.Read(ctx, func(tx *db.Tx) error {
		rows, e := tx.Query("SELECT id FROM node_bootstrap_operations WHERE node_id=? AND operation_type='install' AND status='installed' ORDER BY created_at DESC LIMIT 1", nodeID)
		if e != nil {
			return e
		}
		if len(rows) != 1 {
			return ErrNotFound
		}
		id = rows[0]["id"]
		return nil
	})
	if err != nil {
		return NodeBootstrapOperation{}, err
	}
	return s.NodeBootstrap(ctx, id)
}

func (s *Store) CreateNodeRemoval(ctx context.Context, nodeID, mode string) (NodeBootstrapOperation, error) {
	if mode != "purge" && mode != "detach" {
		return NodeBootstrapOperation{}, ErrConflict
	}
	original, err := s.InstalledBootstrapForNode(ctx, nodeID)
	if err != nil {
		return NodeBootstrapOperation{}, err
	}
	id, err := auth.UUID()
	if err != nil {
		return NodeBootstrapOperation{}, err
	}
	now := time.Now().Unix()
	op := NodeBootstrapOperation{ID: id, Name: original.Name, Address: original.Address, NodeID: nodeID, SSHPort: original.SSHPort, VPNPort: original.VPNPort, Status: "running", Step: "queued", OperationType: "remove", RemovalMode: mode, CreatedAt: now, UpdatedAt: now}
	err = s.DB.WithTx(ctx, func(tx *db.Tx) error {
		exists, e := tx.Query("SELECT id FROM managed_nodes WHERE id=?", nodeID)
		if e != nil {
			return e
		}
		if len(exists) != 1 {
			return ErrNotFound
		}
		if e := requireNodeWithoutRoutes(tx, nodeID); e != nil {
			return e
		}
		running, e := tx.Query("SELECT id FROM node_bootstrap_operations WHERE node_id=? AND status='running' LIMIT 1", nodeID)
		if e != nil {
			return e
		}
		if len(running) != 0 {
			return ErrConflict
		}
		if e := tx.Exec("INSERT INTO node_bootstrap_operations(id,name,address,ssh_port,vpn_port,status,step,created_at,updated_at,node_id,operation_type,removal_mode) VALUES(?,?,?,?,?,'running','queued',?,?,?,?,?)", id, op.Name, op.Address, strconv.Itoa(op.SSHPort), strconv.Itoa(op.VPNPort), strconv.FormatInt(now, 10), strconv.FormatInt(now, 10), nodeID, "remove", mode); e != nil {
			return e
		}
		if e := pruneBootstrapHistory(tx, now); e != nil {
			return e
		}
		return invalidateNodeProbe(tx, nodeID)
	})
	return op, err
}

// CreateNodeUpgrade serializes upgrades with any in-flight SSH removal or
// installation for this managed node. Only the immutable original SSH install
// journal authorizes remote changes; an arbitrary enrolled node is insufficient.
func (s *Store) CreateNodeUpgrade(ctx context.Context, nodeID, targetVersion string) (NodeBootstrapOperation, error) {
	if targetVersion == "" || len(targetVersion) > 64 {
		return NodeBootstrapOperation{}, ErrConflict
	}
	original, err := s.InstalledBootstrapForNode(ctx, nodeID)
	if err != nil {
		return NodeBootstrapOperation{}, err
	}
	id, err := auth.UUID()
	if err != nil {
		return NodeBootstrapOperation{}, err
	}
	now := time.Now().Unix()
	op := NodeBootstrapOperation{ID: id, Name: original.Name, Address: original.Address, NodeID: nodeID,
		SSHPort: original.SSHPort, VPNPort: original.VPNPort, Status: "running", Step: "queued",
		OperationType: "upgrade", TargetVersion: targetVersion, CreatedAt: now, UpdatedAt: now}
	err = s.DB.WithTx(ctx, func(tx *db.Tx) error {
		rows, e := tx.Query("SELECT id FROM managed_nodes WHERE id=?", nodeID)
		if e != nil {
			return e
		}
		if len(rows) != 1 {
			return ErrNotFound
		}
		rows, e = tx.Query("SELECT id FROM node_bootstrap_operations WHERE node_id=? AND status='running' LIMIT 1", nodeID)
		if e != nil {
			return e
		}
		if len(rows) != 0 {
			return ErrConflict
		}
		if e := tx.Exec("INSERT INTO node_bootstrap_operations(id,name,address,ssh_port,vpn_port,status,step,created_at,updated_at,node_id,operation_type,target_version) VALUES(?,?,?,?,?,'running','queued',?,?,?,'upgrade',?)",
			id, op.Name, op.Address, strconv.Itoa(op.SSHPort), strconv.Itoa(op.VPNPort), strconv.FormatInt(now, 10), strconv.FormatInt(now, 10), nodeID, targetVersion); e != nil {
			return e
		}
		if e := pruneBootstrapHistory(tx, now); e != nil {
			return e
		}
		return invalidateNodeProbe(tx, nodeID)
	})
	return op, err
}
