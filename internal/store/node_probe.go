package store

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/arcvpn/arcvpn/internal/auth"
	"github.com/arcvpn/arcvpn/internal/db"
)

// The grant is a single-use, short-lived authority to submit a statement
// about one particular applied configuration revision. It is NOT proof that
// the holder actually ran an external client: a trusted probe runner is part
// of the operational security boundary.
const nodeProbeGrantSchema = `CREATE TABLE IF NOT EXISTS node_probe_grants(
 hash TEXT PRIMARY KEY, node_id TEXT NOT NULL REFERENCES managed_nodes(id) ON DELETE CASCADE,
 revision INTEGER NOT NULL, digest TEXT NOT NULL, expires INTEGER NOT NULL
)`

var ErrProbeGrant = errors.New("invalid or expired probe grant")

func migrateNodeControlProbe(tx *db.Tx) error {
	if err := tx.Exec(nodeProbeGrantSchema); err != nil {
		return err
	}
	if err := tx.Exec("CREATE INDEX IF NOT EXISTS node_probe_grants_node ON node_probe_grants(node_id)"); err != nil {
		return err
	}
	cols, err := tx.Query("PRAGMA table_info(node_control)")
	if err != nil {
		return err
	}
	have := map[string]bool{}
	for _, c := range cols {
		have[c["name"]] = true
	}
	if !have["probe_checked_at"] {
		if err = tx.Exec("ALTER TABLE node_control ADD COLUMN probe_checked_at INTEGER NOT NULL DEFAULT 0"); err != nil {
			return err
		}
	}
	if !have["probe_revision"] {
		if err = tx.Exec("ALTER TABLE node_control ADD COLUMN probe_revision INTEGER NOT NULL DEFAULT 0"); err != nil {
			return err
		}
	}
	return nil
}

type ProbeGrant struct {
	Token     string `json:"token"`
	NodeID    string `json:"node_id"`
	Revision  int64  `json:"revision"`
	Digest    string `json:"digest"`
	ExpiresAt int64  `json:"expires_at"`
}

type ProbeReport struct {
	NodeID     string `json:"node_id"`
	Revision   int64  `json:"revision"`
	Digest     string `json:"digest"`
	Result     string `json:"result"`
	DurationMS int64  `json:"duration_ms"`
}

func (s *Store) CreateProbeGrant(ctx context.Context, id string) (ProbeGrant, error) {
	token, err := auth.Secret(32)
	if err != nil {
		return ProbeGrant{}, err
	}
	var result ProbeGrant
	err = s.DB.WithTx(ctx, func(tx *db.Tx) error {
		rows, e := tx.Query("SELECT * FROM node_control WHERE node_id=?", id)
		if e != nil {
			return e
		}
		if len(rows) != 1 {
			return ErrNotFound
		}
		c := nodeControlFromRow(rows[0])
		if c.Draining || c.Applied <= 0 || c.Applied != c.Desired || c.DeploymentStatus != "applied" || c.Digest == "" {
			return ErrConflict
		}
		expiration := time.Now().Add(5 * time.Minute).Unix()
		result = ProbeGrant{Token: token, NodeID: id, Revision: c.Applied, Digest: c.Digest, ExpiresAt: expiration}
		if e = tx.Exec("DELETE FROM node_probe_grants WHERE node_id=? OR expires<?", id, strconv.FormatInt(time.Now().Unix(), 10)); e != nil {
			return e
		}
		return tx.Exec("INSERT INTO node_probe_grants(hash,node_id,revision,digest,expires) VALUES(?,?,?,?,?)", auth.HashToken(token), id, strconv.FormatInt(c.Applied, 10), c.Digest, strconv.FormatInt(expiration, 10))
	})
	return result, err
}

// CompleteProbe consumes a grant in the same transaction that records the
// outcome. Failed reports never publish nodes. A late/stale attempt never
// marks a newer desired revision healthy.
func (s *Store) CompleteProbe(ctx context.Context, token string, report ProbeReport) (NodeControl, error) {
	if len(token) < 40 || len(token) > 128 || report.NodeID == "" || len(report.NodeID) > 128 || (report.Result != "pass" && report.Result != "fail") || report.DurationMS < 0 || report.DurationMS > 60000 || !revisionToken.MatchString(report.Digest) {
		return NodeControl{}, ErrProbeGrant
	}
	var control NodeControl
	err := s.DB.WithTx(ctx, func(tx *db.Tx) error {
		rows, e := tx.Query("SELECT * FROM node_probe_grants WHERE hash=?", auth.HashToken(token))
		if e != nil {
			return e
		}
		if len(rows) != 1 {
			return ErrProbeGrant
		}
		grant := rows[0]
		expires, _ := strconv.ParseInt(grant["expires"], 10, 64)
		revision, _ := strconv.ParseInt(grant["revision"], 10, 64)
		if expires < time.Now().Unix() || grant["node_id"] != report.NodeID || revision != report.Revision || grant["digest"] != report.Digest {
			return ErrProbeGrant
		}
		current, e := tx.Query("SELECT * FROM node_control WHERE node_id=?", report.NodeID)
		if e != nil {
			return e
		}
		if len(current) != 1 {
			return ErrProbeGrant
		}
		c := nodeControlFromRow(current[0])
		if c.Draining || c.Applied != revision || c.Desired != revision || c.Digest != report.Digest || c.DeploymentStatus != "applied" {
			return ErrConflict
		}
		if e = tx.Exec("DELETE FROM node_probe_grants WHERE hash=?", auth.HashToken(token)); e != nil {
			return e
		}
		now := strconv.FormatInt(time.Now().Unix(), 10)
		if e = tx.Exec("UPDATE node_control SET probe=?,probe_revision=?,probe_checked_at=?,published=0 WHERE node_id=?", report.Result, strconv.FormatInt(revision, 10), now, report.NodeID); e != nil {
			return e
		}
		current, e = tx.Query("SELECT * FROM node_control WHERE node_id=?", report.NodeID)
		if e != nil {
			return e
		}
		control = nodeControlFromRow(current[0])
		return nil
	})
	return control, err
}

// invalidateNodeProbe marks all earlier operational attestations stale before
// replacing binaries, changing node ownership or starting a removal. The
// database transaction also revokes outstanding one-time report grants.
func invalidateNodeProbe(tx *db.Tx, nodeID string) error {
	if err := tx.Exec("DELETE FROM node_probe_grants WHERE node_id=?", nodeID); err != nil {
		return err
	}
	return tx.Exec("UPDATE node_control SET probe='unknown',probe_checked_at=0,probe_revision=0,published=0 WHERE node_id=?", nodeID)
}
