package store

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"syscall"
	"time"

	"github.com/arcvpn/arcvpn/internal/auth"
	"github.com/arcvpn/arcvpn/internal/db"
	"github.com/arcvpn/arcvpn/internal/nodeprotocol"
)

// The control-plane schema is additive. Historical managed_nodes and encrypted
// backup v3 readers continue to work; control-plane secrets are NOT in backups.
const nodeControlSchema = `CREATE TABLE IF NOT EXISTS node_control (
 node_id TEXT PRIMARY KEY REFERENCES managed_nodes(id) ON DELETE CASCADE,
 address TEXT NOT NULL DEFAULT '', port INTEGER NOT NULL DEFAULT 443,
 desired_revision INTEGER NOT NULL DEFAULT 0,
 applied_revision INTEGER NOT NULL DEFAULT 0,
 last_good_revision INTEGER NOT NULL DEFAULT 0,
 desired_digest TEXT NOT NULL DEFAULT '', desired_ciphertext TEXT NOT NULL DEFAULT '',
 deployment_id TEXT NOT NULL DEFAULT '', deployment_status TEXT NOT NULL DEFAULT 'idle',
 last_error TEXT NOT NULL DEFAULT '', updated_at INTEGER NOT NULL DEFAULT 0,
 probe TEXT NOT NULL DEFAULT 'unknown', published INTEGER NOT NULL DEFAULT 0,
 draining INTEGER NOT NULL DEFAULT 0,
 probe_checked_at INTEGER NOT NULL DEFAULT 0, probe_revision INTEGER NOT NULL DEFAULT 0,
 next_secret_hash TEXT NOT NULL DEFAULT '', next_secret_expires INTEGER NOT NULL DEFAULT 0
)`
const nodeOpSchema = `CREATE TABLE IF NOT EXISTS node_operations (
 id TEXT PRIMARY KEY, node_id TEXT NOT NULL REFERENCES managed_nodes(id) ON DELETE CASCADE,
 kind TEXT NOT NULL, revision INTEGER NOT NULL DEFAULT 0, digest TEXT NOT NULL DEFAULT '',
 state TEXT NOT NULL, attempts INTEGER NOT NULL DEFAULT 0,
 created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL, error TEXT NOT NULL DEFAULT ''
)`
const nodeEnrollMetaSchema = `CREATE TABLE IF NOT EXISTS node_enrollment_meta (
 hash TEXT PRIMARY KEY REFERENCES node_enrollments(hash) ON DELETE CASCADE,
 address TEXT NOT NULL DEFAULT '', port INTEGER NOT NULL DEFAULT 443,
 node_id TEXT NOT NULL DEFAULT ''
)`

var revisionToken = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Keep at least the latest 60 operations per node for the administrative UI,
// retain up to 256 terminal operations even during high-frequency rollouts,
// and drop older terminal records after 90 days. The current deployment and
// all nonterminal operations are exempt. Security events use the separate
// application audit log and must not depend on this operational queue.
const nodeOperationRecentLimit = 60
const nodeOperationRetentionLimit = 256
const nodeOperationRetentionPeriod = 90 * 24 * time.Hour

var ErrStaleNodeOperation = errors.New("stale node operation")
var ErrNodeUnauthorized = errors.New("invalid node credentials")
var ErrNodeBusy = errors.New("node deployment already in progress")
var ErrIncompatibleNodeProtocol = errors.New("node agent protocol is incompatible or not reported")

// A bounded number of redeliveries prevents an agent that never ACKs from
// keeping an operation in 'running' indefinitely. The administrator may retry
// the exact same operation identity after this failure.
const maxNodeDeliveryAttempts = 8

// NodeCommand is an intentionally closed, versioned command language. The
// controller cannot run an arbitrary command or specify arbitrary file paths.
type NodeCommand struct {
	ProtocolVersion int             `json:"protocol_version"`
	Kind            string          `json:"kind"`
	OperationID     string          `json:"operation_id"`
	Revision        int64           `json:"revision"`
	SHA256          string          `json:"sha256"`
	Config          json.RawMessage `json:"config"`
}

type NodeOperation struct {
	ID        string `json:"id"`
	NodeID    string `json:"node_id"`
	Kind      string `json:"kind"`
	Revision  int64  `json:"revision"`
	Digest    string `json:"digest"`
	State     string `json:"state"`
	Attempts  int    `json:"attempts"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
	Error     string `json:"error,omitempty"`
}

type NodeControl struct {
	NodeID           string `json:"node_id"`
	Address          string `json:"address"`
	Port             int    `json:"port"`
	Desired          int64  `json:"desired_revision"`
	Applied          int64  `json:"applied_revision"`
	LastGood         int64  `json:"last_good_revision"`
	Digest           string `json:"desired_digest"`
	DeploymentID     string `json:"deployment_id"`
	DeploymentStatus string `json:"deployment_status"`
	LastError        string `json:"last_error,omitempty"`
	UpdatedAt        int64  `json:"updated_at"`
	Probe            string `json:"vpn_probe"`
	ProbeCheckedAt   int64  `json:"probe_checked_at"`
	ProbeRevision    int64  `json:"probe_revision"`
	Published        bool   `json:"published"`
	Draining         bool   `json:"draining"`
	RevisionStatus   string `json:"revision_status"`
}

func (s *Store) MigrateNodeControl(ctx context.Context) error {
	return s.DB.WithTx(ctx, func(tx *db.Tx) error {
		for _, q := range []string{nodeControlSchema, nodeOpSchema, nodeEnrollMetaSchema, nodeProfileSchema, `CREATE INDEX IF NOT EXISTS node_operations_node_idx ON node_operations(node_id,created_at DESC)`, `INSERT OR IGNORE INTO node_control(node_id) SELECT id FROM managed_nodes`} {
			if err := tx.Exec(q); err != nil {
				return err
			}
		}
		if err := migrateNodeControlDrain(tx); err != nil {
			return err
		}
		return migrateNodeControlProbe(tx)
	})
}

// Existing 0.6.1 databases have no draining column. The migration is additive
// and idempotent, preserving all applied revisions and identity records.
func migrateNodeControlDrain(tx *db.Tx) error {
	columns, err := tx.Query("PRAGMA table_info(node_control)")
	if err != nil {
		return err
	}
	for _, column := range columns {
		if column["name"] == "draining" {
			return nil
		}
	}
	return tx.Exec("ALTER TABLE node_control ADD COLUMN draining INTEGER NOT NULL DEFAULT 0")
}

func nodeControlFromRow(row db.Row) NodeControl {
	num := func(k string) int64 { v, _ := strconv.ParseInt(row[k], 10, 64); return v }
	c := NodeControl{NodeID: row["node_id"], Address: row["address"], Port: int(num("port")), Desired: num("desired_revision"), Applied: num("applied_revision"), LastGood: num("last_good_revision"), Digest: row["desired_digest"], DeploymentID: row["deployment_id"], DeploymentStatus: row["deployment_status"], LastError: row["last_error"], UpdatedAt: num("updated_at"), Probe: row["probe"], ProbeCheckedAt: num("probe_checked_at"), ProbeRevision: num("probe_revision"), Published: row["published"] == "1", Draining: row["draining"] == "1"}
	if c.Probe != "unknown" && (c.ProbeRevision != c.Applied || c.Applied != c.Desired || c.ProbeCheckedAt <= 0 || time.Now().Unix()-c.ProbeCheckedAt > 1800) {
		c.Probe = "stale"
	}
	switch {
	case c.DeploymentStatus == "failed":
		c.RevisionStatus = "failed"
	case c.Desired == 0:
		c.RevisionStatus = "none"
	case c.Applied == c.Desired && c.DeploymentStatus == "applied":
		c.RevisionStatus = "applied"
	default:
		c.RevisionStatus = "pending"
	}
	return c
}
func (s *Store) NodeControl(ctx context.Context, id string) (NodeControl, error) {
	var result NodeControl
	err := s.DB.Read(ctx, func(tx *db.Tx) error {
		rows, err := tx.Query("SELECT * FROM node_control WHERE node_id=?", id)
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			return ErrNotFound
		}
		result = nodeControlFromRow(rows[0])
		return nil
	})
	return result, err
}

// SetNodeDraining controls distribution of new diagnostic links only. It does
// not stop Xray, invalidate already exported client URLs, or claim that active
// sessions were drained. Resuming does not publish an unprobed VPN endpoint.
func (s *Store) SetNodeDraining(ctx context.Context, id string, draining bool) (NodeControl, error) {
	var state NodeControl
	err := s.DB.WithTx(ctx, func(tx *db.Tx) error {
		if e := requireNodeWithoutCascade(tx, id); e != nil {
			return e
		}
		rows, e := tx.Query("SELECT * FROM node_control WHERE node_id=?", id)
		if e != nil {
			return e
		}
		if len(rows) != 1 {
			return ErrNotFound
		}
		previous := nodeControlFromRow(rows[0])
		// Repeated requests can arrive after a lost HTTP response. A no-op
		// resume must not unexpectedly unpublish an existing endpoint.
		// A repeated drain still enforces unpublished state.
		if previous.Draining == draining && (!draining || !previous.Published) {
			state = previous
			return nil
		}
		if !draining && previous.Draining && (previous.Applied == 0 || previous.Applied != previous.Desired || previous.DeploymentStatus != "applied") {
			return ErrConflict
		}
		flag := "0"
		if draining {
			flag = "1"
		}
		if draining {
			if e = tx.Exec("DELETE FROM node_probe_grants WHERE node_id=?", id); e != nil {
				return e
			}
		}
		if e = tx.Exec("UPDATE node_control SET draining=?,published=0,updated_at=? WHERE node_id=?", flag, strconv.FormatInt(time.Now().Unix(), 10), id); e != nil {
			return e
		}
		rows, e = tx.Query("SELECT * FROM node_control WHERE node_id=?", id)
		if e != nil {
			return e
		}
		state = nodeControlFromRow(rows[0])
		return nil
	})
	return state, err
}

func (s *Store) NodeOperations(ctx context.Context, id string) ([]NodeOperation, error) {
	out := make([]NodeOperation, 0)
	err := s.DB.Read(ctx, func(tx *db.Tx) error {
		rows, e := tx.Query("SELECT * FROM node_operations WHERE node_id=? ORDER BY created_at DESC,id DESC LIMIT 60", id)
		if e != nil {
			return e
		}
		for _, r := range rows {
			rev, _ := strconv.ParseInt(r["revision"], 10, 64)
			att, _ := strconv.Atoi(r["attempts"])
			created, _ := strconv.ParseInt(r["created_at"], 10, 64)
			updated, _ := strconv.ParseInt(r["updated_at"], 10, 64)
			out = append(out, NodeOperation{ID: r["id"], NodeID: r["node_id"], Kind: r["kind"], Revision: rev, Digest: r["digest"], State: r["state"], Attempts: att, CreatedAt: created, UpdatedAt: updated, Error: r["error"]})
		}
		return nil
	})
	return out, err
}

func (s *Store) enrollmentDetails(ctx context.Context, token, nodeID, address string, port int) (string, error) {
	// The caller has validated address and port. Both tables use one transaction.
	tokenHash := auth.HashToken(token)
	err := s.DB.WithTx(ctx, func(tx *db.Tx) error {
		return tx.Exec("INSERT INTO node_enrollment_meta(hash,address,port,node_id) VALUES(?,?,?,?)", tokenHash, address, strconv.Itoa(port), nodeID)
	})
	return token, err
}

// Seal configs with an independent 256-bit key stored in a 0600 file beside the
// database. Backups intentionally exclude it: restore REQUIRES explicit rebind.
// Never auto-create a replacement key if ciphertext already exists.
// initializeNodeKey is deliberately called outside the database transaction.
// If the key disappeared while encrypted rows still exist, fail closed.
func (s *Store) initializeNodeKey(ctx context.Context) error {
	path := filepath.Join(filepath.Dir(s.DB.Path()), "node-control.key")
	if _, err := os.Stat(path); err == nil {
		_, err = s.nodeControlKey()
		return err
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	var count int
	if err := s.DB.Read(ctx, func(tx *db.Tx) error {
		for _, query := range []string{"SELECT COUNT(*) AS n FROM node_control WHERE desired_ciphertext<>''", "SELECT COUNT(*) AS n FROM node_profiles"} {
			rows, err := tx.Query(query)
			if err != nil {
				return err
			}
			n, _ := strconv.Atoi(rows[0]["n"])
			count += n
		}
		return nil
	}); err != nil {
		return err
	}
	if count != 0 {
		return errors.New("node encryption key missing: encrypted node state cannot be restored")
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return err
	}
	created, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if errors.Is(err, os.ErrExist) {
		_, err = s.nodeControlKey()
		return err
	}
	if err != nil {
		return err
	}
	_, err = created.Write(key)
	if err == nil {
		err = created.Sync()
	}
	closeErr := created.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(path)
		return err
	}
	return nil
}
func (s *Store) nodeControlKey() ([]byte, error) {
	path := filepath.Join(filepath.Dir(s.DB.Path()), "node-control.key")
	// The encryption key is a trust anchor: open the inode only once and
	// refuse symlinks/FIFOs, including if the path is swapped concurrently.
	// O_NONBLOCK prevents a FIFO substituted at this path from hanging the
	// controller before Stat can reject it.
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, errors.New("cannot securely open node encryption key")
	}
	f := os.NewFile(uintptr(fd), path)
	defer f.Close()
	stat, err := f.Stat()
	if err != nil || !stat.Mode().IsRegular() || stat.Mode().Perm()&0077 != 0 || stat.Size() != 32 {
		return nil, errors.New("insecure node key permissions or size")
	}
	key, err := io.ReadAll(io.LimitReader(f, 33))
	if err != nil {
		return nil, err
	}
	if len(key) != 32 {
		return nil, errors.New("invalid node encryption key")
	}
	return key, nil
}
func (s *Store) sealNodeConfig(plaintext []byte, ad string) (string, error) {
	key, e := s.nodeControlKey()
	if e != nil {
		return "", e
	}
	block, e := aes.NewCipher(key)
	if e != nil {
		return "", e
	}
	gcm, e := cipher.NewGCM(block)
	if e != nil {
		return "", e
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, e = rand.Read(nonce); e != nil {
		return "", e
	}
	return hex.EncodeToString(gcm.Seal(nonce, nonce, plaintext, []byte(ad))), nil
}
func (s *Store) openNodeConfig(sealed, ad string) ([]byte, error) {
	raw, e := hex.DecodeString(sealed)
	if e != nil {
		return nil, e
	}
	key, e := s.nodeControlKey()
	if e != nil {
		return nil, e
	}
	block, e := aes.NewCipher(key)
	if e != nil {
		return nil, e
	}
	gcm, e := cipher.NewGCM(block)
	if e != nil {
		return nil, e
	}
	if len(raw) < gcm.NonceSize() {
		return nil, errors.New("short sealed config")
	}
	return gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], []byte(ad))
}

// QueueNodeDeployment replaces the desired config only while the previous
// operation is terminal, ensuring a serial, monotonic per-node revision.
func (s *Store) QueueNodeDeployment(ctx context.Context, id string, config []byte) (NodeOperation, error) {
	var op NodeOperation
	err := s.DB.WithTx(ctx, func(tx *db.Tx) error {
		if err := requireNodeWithoutCascade(tx, id); err != nil {
			return err
		}
		var err error
		op, err = s.queueNodeDeployment(tx, id, config, "apply_xray_v1")
		return err
	})
	return op, err
}
func (s *Store) queueNodeDeployment(tx *db.Tx, id string, config []byte, kind string) (NodeOperation, error) {
	if kind != "apply_xray_v1" && kind != "apply_cascade_xray_v1" && kind != "apply_xray_devices_v1" && kind != "apply_xray_usage_v1" {
		return NodeOperation{}, ErrRouteInvalid
	}
	if len(config) == 0 || len(config) > 256<<10 || !json.Valid(config) {
		return NodeOperation{}, errors.New("invalid Xray JSON")
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, config); err != nil {
		return NodeOperation{}, err
	}
	config = compact.Bytes()
	hash := sha256.Sum256(config)
	digest := hex.EncodeToString(hash[:])
	operationID, e := auth.UUID()
	if e != nil {
		return NodeOperation{}, e
	}
	var op NodeOperation
	rows, e := tx.Query("SELECT desired_revision,deployment_status FROM node_control WHERE node_id=?", id)
	if e != nil {
		return NodeOperation{}, e
	}
	if len(rows) == 0 {
		return NodeOperation{}, ErrNotFound
	}
	if rows[0]["deployment_status"] == "queued" || rows[0]["deployment_status"] == "running" {
		return NodeOperation{}, ErrNodeBusy
	}
	previous, parseErr := strconv.ParseInt(rows[0]["desired_revision"], 10, 64)
	if parseErr != nil || previous < 0 || previous == math.MaxInt64 {
		return NodeOperation{}, ErrConflict
	}
	rev := previous + 1
	sealed, e := s.sealNodeConfig(config, fmt.Sprintf("%s:%d:%s", id, rev, digest))
	if e != nil {
		return NodeOperation{}, e
	}
	now := time.Now().Unix()
	op = NodeOperation{ID: operationID, NodeID: id, Kind: kind, Revision: rev, Digest: digest, State: "queued", CreatedAt: now, UpdatedAt: now}
	if e = tx.Exec("UPDATE node_control SET desired_revision=?,desired_digest=?,desired_ciphertext=?,deployment_id=?,deployment_status='queued',last_error='',probe='unknown',probe_revision=0,probe_checked_at=0,published=0,updated_at=? WHERE node_id=?", strconv.FormatInt(rev, 10), digest, sealed, operationID, strconv.FormatInt(now, 10), id); e != nil {
		return NodeOperation{}, e
	}
	if e = tx.Exec("INSERT INTO node_operations(id,node_id,kind,revision,digest,state,created_at,updated_at) VALUES(?,?,?, ?,?,'queued',?,?)", operationID, id, op.Kind, strconv.FormatInt(rev, 10), digest, strconv.FormatInt(now, 10), strconv.FormatInt(now, 10)); e != nil {
		return NodeOperation{}, e
	}
	return op, pruneNodeOperations(tx, id, now)
}

// pruneNodeOperations is transactional with creating a new deployment. Never
// discard the most recent operations, a live operation, or the operation ID
// needed to reconcile a lost ACK. It is intentionally scoped to one node.
func pruneNodeOperations(tx *db.Tx, nodeID string, now int64) error {
	cutoff := strconv.FormatInt(now-int64(nodeOperationRetentionPeriod/time.Second), 10)
	return tx.Exec(`DELETE FROM node_operations
		WHERE node_id=? AND state IN ('applied','failed','cancelled')
		AND id NOT IN (SELECT deployment_id FROM node_control WHERE node_id=?)
		AND id NOT IN (
			SELECT id FROM node_operations WHERE node_id=?
			ORDER BY created_at DESC, id DESC LIMIT ?
		)
		AND (
			updated_at < ? OR id NOT IN (
				SELECT id FROM node_operations WHERE node_id=?
				ORDER BY created_at DESC, id DESC LIMIT ?
			)
		)`, nodeID, nodeID, nodeID, strconv.Itoa(nodeOperationRecentLimit), cutoff, nodeID, strconv.Itoa(nodeOperationRetentionLimit))
}

func (s *Store) nodeForSecret(tx *db.Tx, secret, nodeID string) (bool, error) {
	if len(secret) < 40 || len(secret) > 128 || len(nodeID) < 1 || len(nodeID) > 128 {
		return false, nil
	}
	rows, e := tx.Query("SELECT token_hash FROM managed_nodes WHERE id=?", nodeID)
	if e != nil {
		return false, e
	}
	if len(rows) != 1 {
		return false, nil
	}
	hash := auth.HashToken(secret)
	if subtle.ConstantTimeCompare([]byte(rows[0]["token_hash"]), []byte(hash)) == 1 {
		return true, nil
	}
	other, e := tx.Query("SELECT next_secret_hash,next_secret_expires FROM node_control WHERE node_id=?", nodeID)
	if e != nil {
		return false, e
	}
	if len(other) != 1 {
		return false, nil
	}
	until, _ := strconv.ParseInt(other[0]["next_secret_expires"], 10, 64)
	return until >= time.Now().Unix() && subtle.ConstantTimeCompare([]byte(other[0]["next_secret_hash"]), []byte(hash)) == 1, nil
}
func (s *Store) PendingNodeCommand(ctx context.Context, nodeID, secret string) (*NodeCommand, error) {
	var cmd *NodeCommand
	e := s.DB.WithTx(ctx, func(tx *db.Tx) error {
		authenticated, e := s.nodeForSecret(tx, secret, nodeID)
		if e != nil {
			return e
		}
		if !authenticated {
			return ErrNodeUnauthorized
		}
		rows, e := tx.Query("SELECT * FROM node_control WHERE node_id=?", nodeID)
		if e != nil {
			return e
		}
		if len(rows) != 1 {
			return ErrNotFound
		}
		r := rows[0]
		protocolRows, e := tx.Query("SELECT agent_protocol FROM managed_nodes WHERE id=?", nodeID)
		if e != nil {
			return e
		}
		if len(protocolRows) != 1 {
			return ErrNotFound
		}
		protocol, e := strconv.Atoi(protocolRows[0]["agent_protocol"])
		if e != nil || !nodeprotocol.Supports(protocol) {
			return ErrIncompatibleNodeProtocol
		}
		if r["deployment_status"] != "queued" && r["deployment_status"] != "running" {
			return nil
		}
		attemptRows, e := tx.Query("SELECT attempts,kind FROM node_operations WHERE id=?", r["deployment_id"])
		if e != nil {
			return e
		}
		if len(attemptRows) != 1 {
			return errors.New("missing deployment journal")
		}
		if attemptRows[0]["kind"] == "apply_xray_usage_v1" {
			if e := requireNodeUsage(tx, nodeID); e != nil {
				return e
			}
		}
		if attemptRows[0]["kind"] == "apply_xray_devices_v1" {
			if e := requireDeviceNode(tx, nodeID); e != nil {
				now := strconv.FormatInt(time.Now().Unix(), 10)
				if e := tx.Exec("UPDATE node_control SET deployment_status='failed',last_error='device helper capability unavailable',updated_at=? WHERE node_id=?", now, nodeID); e != nil {
					return e
				}
				return tx.Exec("UPDATE node_operations SET state='failed',error='device helper capability unavailable',updated_at=? WHERE id=?", now, r["deployment_id"])
			}
		}
		if attemptRows[0]["kind"] == "apply_cascade_xray_v1" {
			caps, e := tx.Query("SELECT protocol,checked_at FROM cascade_capabilities WHERE node_id=?", nodeID)
			if e != nil {
				return e
			}
			checked := int64(0)
			if len(caps) == 1 {
				checked, _ = strconv.ParseInt(caps[0]["checked_at"], 10, 64)
			}
			if len(caps) != 1 || caps[0]["protocol"] != "1" || time.Now().Unix()-checked > 180 {
				now := strconv.FormatInt(time.Now().Unix(), 10)
				if e := tx.Exec("UPDATE node_control SET deployment_status='failed',last_error='cascade helper capability expired or was withdrawn',updated_at=? WHERE node_id=?", now, nodeID); e != nil {
					return e
				}
				if e := tx.Exec("UPDATE node_operations SET state='failed',error='cascade helper capability unavailable',updated_at=? WHERE id=?", now, r["deployment_id"]); e != nil {
					return e
				}
				return markCascadeOperationUnknown(tx, r["deployment_id"])
			}
		}

		attempts, e := strconv.Atoi(attemptRows[0]["attempts"])
		if e != nil {
			return errors.New("invalid attempt count")
		}
		if attempts >= maxNodeDeliveryAttempts {
			now := strconv.FormatInt(time.Now().Unix(), 10)
			if e = tx.Exec("UPDATE node_control SET deployment_status='failed',last_error='agent did not acknowledge after bounded retries',updated_at=? WHERE node_id=?", now, nodeID); e != nil {
				return e
			}
			if e := tx.Exec("UPDATE node_operations SET state='failed',error='agent did not acknowledge after bounded retries',updated_at=? WHERE id=?", now, r["deployment_id"]); e != nil {
				return e
			}
			return markCascadeOperationUnknown(tx, r["deployment_id"])
		}
		rev, _ := strconv.ParseInt(r["desired_revision"], 10, 64)
		plain, e := s.openNodeConfig(r["desired_ciphertext"], fmt.Sprintf("%s:%d:%s", nodeID, rev, r["desired_digest"]))
		if e != nil {
			return e
		}
		hash := sha256.Sum256(plain)
		if hex.EncodeToString(hash[:]) != r["desired_digest"] {
			return errors.New("node config digest mismatch")
		}
		now := strconv.FormatInt(time.Now().Unix(), 10)
		if e = tx.Exec("UPDATE node_control SET deployment_status='running',updated_at=? WHERE node_id=?", now, nodeID); e != nil {
			return e
		}
		if e = tx.Exec("UPDATE node_operations SET state='running',attempts=attempts+1,updated_at=? WHERE id=?", now, r["deployment_id"]); e != nil {
			return e
		}
		cmd = &NodeCommand{ProtocolVersion: 1, Kind: attemptRows[0]["kind"], OperationID: r["deployment_id"], Revision: rev, SHA256: r["desired_digest"], Config: json.RawMessage(plain)}
		return nil
	})
	return cmd, e
}

func (s *Store) AcknowledgeNodeOperation(ctx context.Context, nodeID, secret, operationID string, revision int64, digest, result string) error {
	if result != "applied" && result != "failed" && result != "rolled_back" || revision <= 0 || !revisionToken.MatchString(digest) {
		return errors.New("invalid acknowledgment")
	}
	return s.DB.WithTx(ctx, func(tx *db.Tx) error {
		authenticated, e := s.nodeForSecret(tx, secret, nodeID)
		if e != nil {
			return e
		}
		if !authenticated {
			return ErrNodeUnauthorized
		}
		rows, e := tx.Query("SELECT desired_revision,applied_revision,desired_digest,deployment_id,deployment_status FROM node_control WHERE node_id=?", nodeID)
		if e != nil {
			return e
		}
		if len(rows) == 0 {
			return ErrNotFound
		}
		r := rows[0]
		desired, _ := strconv.ParseInt(r["desired_revision"], 10, 64)
		applied, _ := strconv.ParseInt(r["applied_revision"], 10, 64)
		if r["deployment_id"] != operationID || desired != revision || r["desired_digest"] != digest {
			return ErrStaleNodeOperation
		}
		if r["deployment_status"] == "applied" && result == "applied" && applied == revision {
			return nil
		}
		if r["deployment_status"] != "running" && r["deployment_status"] != "queued" {
			return ErrStaleNodeOperation
		}
		status := result
		errorText := ""
		if status == "rolled_back" || status == "failed" {
			status = "failed"
			errorText = "node rejected the desired configuration; previous VPN state preserved when available"
		}
		now := strconv.FormatInt(time.Now().Unix(), 10)
		if result == "applied" {
			if revision <= applied {
				return ErrStaleNodeOperation
			}
			if e = tx.Exec("UPDATE node_control SET applied_revision=?,last_good_revision=?,deployment_status='applied',last_error='',updated_at=? WHERE node_id=?", strconv.FormatInt(revision, 10), strconv.FormatInt(revision, 10), now, nodeID); e != nil {
				return e
			}
		} else {
			if e = tx.Exec("UPDATE node_control SET deployment_status='failed',last_error=?,updated_at=? WHERE node_id=?", errorText, now, nodeID); e != nil {
				return e
			}
		}
		if e := tx.Exec("UPDATE node_operations SET state=?,error=?,updated_at=? WHERE id=?", status, errorText, now, operationID); e != nil {
			return e
		}
		if result == "applied" {
			if e := tx.Exec("UPDATE node_credentials SET applied_generation=generation WHERE node_id=? AND operation_id=?", nodeID, operationID); e != nil {
				return e
			}
		}
		return s.advanceCascadeACK(tx, operationID, result)
	})
}

// RetryNodeDeployment requeues the *same operation ID*. A node may have
// successfully applied it before its ACK was lost. Reusing the ID lets the
// root helper return its durable receipt instead of treating a new ID for the
// same revision as an unauthorized replay.
func (s *Store) RetryNodeDeployment(ctx context.Context, nodeID string) (NodeOperation, error) {
	var op NodeOperation
	e := s.DB.WithTx(ctx, func(tx *db.Tx) error {
		if e := requireNodeWithoutCascade(tx, nodeID); e != nil {
			return e
		}
		rows, e := tx.Query("SELECT desired_revision,desired_digest,deployment_status,deployment_id FROM node_control WHERE node_id=?", nodeID)
		if e != nil {
			return e
		}
		if len(rows) == 0 {
			return ErrNotFound
		}
		r := rows[0]
		if r["deployment_status"] != "failed" {
			return ErrConflict
		}
		id := r["deployment_id"]
		if id == "" {
			return ErrConflict
		}
		old, e := tx.Query("SELECT created_at,kind FROM node_operations WHERE id=? AND node_id=?", id, nodeID)
		if e != nil {
			return e
		}
		if len(old) != 1 {
			return ErrConflict
		}
		now := time.Now().Unix()
		rev, _ := strconv.ParseInt(r["desired_revision"], 10, 64)
		created, _ := strconv.ParseInt(old[0]["created_at"], 10, 64)
		op = NodeOperation{ID: id, NodeID: nodeID, Kind: old[0]["kind"], Revision: rev, Digest: r["desired_digest"], State: "queued", CreatedAt: created, UpdatedAt: now}
		if e = tx.Exec("UPDATE node_control SET deployment_status='queued',last_error='',updated_at=? WHERE node_id=?", strconv.FormatInt(now, 10), nodeID); e != nil {
			return e
		}
		return tx.Exec("UPDATE node_operations SET state='queued',attempts=0,error='',updated_at=? WHERE id=? AND node_id=?", strconv.FormatInt(now, 10), id, nodeID)
	})
	return op, e
}
func (s *Store) MarkNodeStalled(ctx context.Context, olderThan time.Duration) error {
	now := time.Now().Unix()
	cutoff := now - int64(olderThan.Seconds())
	return s.DB.WithTx(ctx, func(tx *db.Tx) error {
		rows, e := tx.Query("SELECT deployment_id,node_id FROM node_control WHERE deployment_status='running' AND updated_at<?", strconv.FormatInt(cutoff, 10))
		if e != nil {
			return e
		}
		for _, r := range rows {
			if e = tx.Exec("UPDATE node_control SET deployment_status='failed',last_error='agent did not acknowledge before deadline',updated_at=? WHERE node_id=?", strconv.FormatInt(now, 10), r["node_id"]); e != nil {
				return e
			}
			if e = tx.Exec("UPDATE node_operations SET state='failed',error='agent did not acknowledge before deadline',updated_at=? WHERE id=?", strconv.FormatInt(now, 10), r["deployment_id"]); e != nil {
				return e
			}
			if e := markCascadeOperationUnknown(tx, r["deployment_id"]); e != nil {
				return e
			}
		}
		return nil
	})
}
func (s *Store) CancelNodeDeployment(ctx context.Context, nodeID string) error {
	return s.DB.WithTx(ctx, func(tx *db.Tx) error {
		if e := requireNodeWithoutCascade(tx, nodeID); e != nil {
			return e
		}
		rows, e := tx.Query("SELECT deployment_id,deployment_status FROM node_control WHERE node_id=?", nodeID)
		if e != nil {
			return e
		}
		if len(rows) == 0 {
			return ErrNotFound
		}
		if rows[0]["deployment_status"] != "queued" {
			return ErrConflict
		} // cannot cancel a possibly executing root-helper job
		now := strconv.FormatInt(time.Now().Unix(), 10)
		if e = tx.Exec("UPDATE node_control SET deployment_status='cancelled',last_error='',updated_at=? WHERE node_id=?", now, nodeID); e != nil {
			return e
		}
		return tx.Exec("UPDATE node_operations SET state='cancelled',updated_at=? WHERE id=?", now, rows[0]["deployment_id"])
	})
}
func (s *Store) RotateNodeCredential(ctx context.Context, nodeID, secret string) (string, error) {
	next, e := auth.Secret(32)
	if e != nil {
		return "", e
	}
	e = s.DB.WithTx(ctx, func(tx *db.Tx) error {
		ok, e := s.nodeForSecret(tx, secret, nodeID)
		if e != nil {
			return e
		}
		if !ok {
			return ErrNodeUnauthorized
		}
		rows, e := tx.Query("SELECT next_secret_hash,next_secret_expires FROM node_control WHERE node_id=?", nodeID)
		if e != nil {
			return e
		}
		if len(rows) == 0 {
			return ErrNotFound
		}
		expire, _ := strconv.ParseInt(rows[0]["next_secret_expires"], 10, 64)
		if expire > time.Now().Unix() {
			return ErrConflict
		}
		return tx.Exec("UPDATE node_control SET next_secret_hash=?,next_secret_expires=? WHERE node_id=?", auth.HashToken(next), strconv.FormatInt(time.Now().Add(3*time.Minute).Unix(), 10), nodeID)
	})
	return next, e
}
func (s *Store) ConfirmNodeCredential(ctx context.Context, nodeID, secret string) error {
	return s.DB.WithTx(ctx, func(tx *db.Tx) error {
		rows, e := tx.Query("SELECT next_secret_hash,next_secret_expires FROM node_control WHERE node_id=?", nodeID)
		if e != nil {
			return e
		}
		if len(rows) == 0 {
			return ErrNotFound
		}
		expire, _ := strconv.ParseInt(rows[0]["next_secret_expires"], 10, 64)
		if expire < time.Now().Unix() || subtle.ConstantTimeCompare([]byte(rows[0]["next_secret_hash"]), []byte(auth.HashToken(secret))) != 1 {
			return ErrNodeUnauthorized
		}
		if e = tx.Exec("UPDATE managed_nodes SET token_hash=? WHERE id=?", auth.HashToken(secret), nodeID); e != nil {
			return e
		}
		return tx.Exec("UPDATE node_control SET next_secret_hash='',next_secret_expires=0 WHERE node_id=?", nodeID)
	})
}

func (s *Store) NodeRevokeEnrollment(ctx context.Context, token string) error {
	return s.DB.WithTx(ctx, func(tx *db.Tx) error {
		return tx.Exec("DELETE FROM node_enrollments WHERE hash=?", auth.HashToken(token))
	})
}
