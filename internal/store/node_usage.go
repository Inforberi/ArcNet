package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"

	"github.com/arcvpn/arcvpn/internal/db"
	"github.com/arcvpn/arcvpn/internal/usage"
)

func migrateNodeUsage(tx *db.Tx) error {
	for _, q := range []string{
		`CREATE INDEX IF NOT EXISTS traffic_checkpoints_stream_idx ON traffic_checkpoints(stream)`,
		`CREATE TABLE IF NOT EXISTS node_usage_capabilities(node_id TEXT PRIMARY KEY REFERENCES managed_nodes(id) ON DELETE CASCADE,protocol INTEGER NOT NULL,checked_at INTEGER NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS node_usage_state(node_id TEXT PRIMARY KEY REFERENCES managed_nodes(id) ON DELETE CASCADE,epoch TEXT NOT NULL,started_at INTEGER NOT NULL,observed_at INTEGER NOT NULL)`,
	} {
		if e := tx.Exec(q); e != nil {
			return e
		}
	}
	return nil
}

func requireNodeUsage(tx *db.Tx, node string) error {
	rows, e := tx.Query("SELECT protocol,checked_at FROM node_usage_capabilities WHERE node_id=?", node)
	if e != nil {
		return e
	}
	if len(rows) != 1 || rows[0]["protocol"] != "1" {
		return ErrUsageUnsupported
	}
	checked, e := strconv.ParseInt(rows[0]["checked_at"], 10, 64)
	if e != nil || checked > time.Now().Unix()+5 || time.Now().Unix()-checked > 75 {
		return ErrUsageUnsupported
	}
	return nil
}

func (s *Store) NodeUsageStatus(ctx context.Context, node string) (capable, configured bool, err error) {
	err = s.DB.Read(ctx, func(tx *db.Tx) error {
		if e := requireNodeWithoutCascade(tx, node); e != nil {
			if e == ErrCascadeBusy {
				return nil
			}
			return e
		}
		if e := requireNodeUsage(tx, node); e != nil {
			if e == ErrUsageUnsupported {
				return nil
			}
			return e
		}
		capable = true
		rows, e := tx.Query("SELECT o.kind FROM node_control c JOIN node_operations o ON o.id=c.deployment_id WHERE c.node_id=?", node)
		if e != nil {
			return e
		}
		configured = len(rows) == 1 && rows[0]["kind"] == "apply_xray_usage_v1"
		return nil
	})
	return
}

// Authentication, exact applied revision, roster, deltas and debit totals share
// one transaction. A heartbeat or report from another node proves none of these.
func (s *Store) RecordNodeUsage(ctx context.Context, node, secret string, r usage.NodeReport) error {
	now := time.Now()
	if r.Version != 1 || r.NodeID != node || r.Revision < 1 || r.ObservedAt < now.Unix()-usage.FreshFor || r.ObservedAt > now.Unix()+5 || r.StartedAt < 1 || r.StartedAt > now.Add(5*time.Second).UnixNano() || len(r.Epoch) < 1 || len(r.Epoch) > 128 || len(r.Counters) > usage.MaxNodeCounters {
		return usage.ErrInvalid
	}
	return s.DB.WithTx(ctx, func(tx *db.Tx) error {
		ok, e := s.nodeForSecret(tx, secret, node)
		if e != nil {
			return e
		}
		if !ok {
			return ErrNodeUnauthorized
		}
		if e = requireNodeWithoutCascade(tx, node); e != nil {
			return e
		}
		if e = requireNodeUsage(tx, node); e != nil {
			return e
		}
		rows, e := tx.Query("SELECT c.*,o.kind FROM node_control c JOIN node_operations o ON o.id=c.deployment_id WHERE c.node_id=?", node)
		if e != nil {
			return e
		}
		if len(rows) != 1 {
			return ErrConflict
		}
		state := rows[0]
		if state["kind"] != "apply_xray_usage_v1" || state["deployment_status"] != "applied" || state["desired_revision"] != state["applied_revision"] || state["desired_revision"] != strconv.FormatInt(r.Revision, 10) || state["deployment_id"] != r.OperationID || state["desired_digest"] != r.Digest {
			return ErrConflict
		}
		config, e := s.openNodeConfig(state["desired_ciphertext"], fmt.Sprintf("%s:%d:%s", node, r.Revision, r.Digest))
		if e != nil {
			return e
		}
		hash := sha256.Sum256(config)
		if hex.EncodeToString(hash[:]) != r.Digest {
			return ErrConflict
		}
		clients, e := usage.NodeClients(config)
		if e != nil {
			return e
		}
		if len(clients) != len(r.Counters) {
			return usage.ErrInvalid
		}
		previous, e := tx.Query("SELECT epoch,started_at,observed_at FROM node_usage_state WHERE node_id=?", node)
		if e != nil {
			return e
		}
		if len(previous) == 1 {
			p := previous[0]
			started, _ := strconv.ParseInt(p["started_at"], 10, 64)
			observed, _ := strconv.ParseInt(p["observed_at"], 10, 64)
			if r.StartedAt < started || (r.Epoch != p["epoch"] && r.StartedAt <= started) || (r.Epoch == p["epoch"] && r.StartedAt != started) {
				return usage.ErrInvalid
			}
			if r.ObservedAt < observed {
				return nil
			}
		}
		users, e := tx.Query("SELECT id,uuid FROM users")
		if e != nil {
			return e
		}
		identities := map[int64]string{}
		for _, u := range users {
			id, _ := strconv.ParseInt(u["id"], 10, 64)
			identities[id] = u["uuid"]
		}
		devices, e := readDevices(tx)
		if e != nil {
			return e
		}
		// The encrypted roster must still belong to these users. Device records
		// remain after revoke, so the measured tail is charged to its real owner.
		credentials := map[int64]map[string]bool{}
		for id, identity := range identities {
			credentials[id] = map[string]bool{identity: true}
		}
		for _, record := range devices {
			for _, d := range record.Devices {
				if credentials[record.UserID] != nil {
					credentials[record.UserID][d.UUID] = true
				}
			}
		}
		seen := map[string]bool{}
		totals := map[int64]usage.Counter{}
		for _, c := range r.Counters {
			owner, ok := clients[c.Name]
			if !ok || seen[c.Name] || !credentials[owner.UserID][owner.Credential] || c.Upload < 0 || c.Download < 0 || c.Upload > usage.MaxBytes || c.Download > usage.MaxBytes-c.Upload {
				return usage.ErrInvalid
			}
			seen[c.Name] = true
			t := totals[owner.UserID]
			if c.Upload > usage.MaxBytes-t.Upload-t.Download || c.Download > usage.MaxBytes-t.Upload-t.Download-c.Upload {
				return usage.ErrInvalid
			}
			t.Source = node
			t.UserID = owner.UserID
			t.Identity = identities[owner.UserID]
			t.Stream = "vless-reality"
			t.Epoch = r.Epoch
			t.StartedAt = r.StartedAt
			t.ObservedAt = r.ObservedAt
			t.Upload += c.Upload
			t.Download += c.Download
			totals[owner.UserID] = t
		}
		counters := make([]usage.Counter, 0, len(totals))
		for _, c := range totals {
			counters = append(counters, c)
		}
		if e = recordCounters(tx, "node_"+node, counters, r.ObservedAt, ""); e != nil {
			return e
		}
		return tx.Exec("INSERT INTO node_usage_state(node_id,epoch,started_at,observed_at) VALUES(?,?,?,?) ON CONFLICT(node_id) DO UPDATE SET epoch=excluded.epoch,started_at=excluded.started_at,observed_at=excluded.observed_at", node, r.Epoch, strconv.FormatInt(r.StartedAt, 10), strconv.FormatInt(r.ObservedAt, 10))
	})
}
