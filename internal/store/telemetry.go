package store

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/arcvpn/arcvpn/internal/db"
	"github.com/arcvpn/arcvpn/internal/metrics"
)

const telemetrySchema = `CREATE TABLE IF NOT EXISTS telemetry_samples(node_id TEXT NOT NULL,bucket INTEGER NOT NULL,received_at INTEGER NOT NULL,body TEXT NOT NULL,PRIMARY KEY(node_id,bucket))`
const telemetryTimeIndex = `CREATE INDEX IF NOT EXISTS telemetry_samples_time ON telemetry_samples(bucket DESC)`

var ErrTelemetryRateLimit = errors.New("telemetry rate limited")

type ObservedTelemetry struct {
	ReceivedAt int64            `json:"received_at"`
	Snapshot   metrics.Snapshot `json:"snapshot"`
}

func (s *Store) RecordTelemetry(ctx context.Context, nodeID, secret string, snapshot metrics.Snapshot) error {
	if nodeID == "" || nodeID == "local" {
		return ErrNodeUnauthorized
	}
	if err := snapshot.Validate(); err != nil {
		return err
	}
	now := time.Now().Unix()
	if snapshot.SampledAt < now-300 || snapshot.SampledAt > now+300 {
		return errors.New("telemetry clock outside permitted window")
	}
	return s.DB.WithTx(ctx, func(tx *db.Tx) error {
		allowed, err := s.nodeForSecret(tx, secret, nodeID)
		if err != nil {
			return err
		}
		if !allowed {
			return ErrNodeUnauthorized
		}
		return writeTelemetry(tx, nodeID, snapshot, now, true)
	})
}
func (s *Store) RecordLocalTelemetry(ctx context.Context, snapshot metrics.Snapshot) error {
	if err := snapshot.Validate(); err != nil {
		return err
	}
	return s.DB.WithTx(ctx, func(tx *db.Tx) error { return writeTelemetry(tx, "local", snapshot, time.Now().Unix(), false) })
}
func writeTelemetry(tx *db.Tx, nodeID string, snapshot metrics.Snapshot, now int64, strict bool) error {
	rows, err := tx.Query("SELECT received_at,bucket,body FROM telemetry_samples WHERE node_id=? ORDER BY bucket DESC LIMIT 1", nodeID)
	if err != nil {
		return err
	}
	if len(rows) > 0 {
		last, err := strconv.ParseInt(rows[0]["received_at"], 10, 64)
		if err != nil {
			return err
		}
		minimumInterval := int64(10)
		if !strict {
			minimumInterval = int64(metrics.CollectionInterval / time.Second)
		}
		if now-last < minimumInterval && now >= last {
			if strict {
				return ErrTelemetryRateLimit
			}
			return nil
		}
	}
	if strict {
		snapshot.RXPerSecond = nil
		snapshot.TXPerSecond = nil
		if len(rows) > 0 {
			var previous metrics.Snapshot
			if err = json.Unmarshal([]byte(rows[0]["body"]), &previous); err != nil {
				return err
			}
			dt := snapshot.SampledAt - previous.SampledAt
			if snapshot.Epoch != "" && snapshot.Epoch == previous.Epoch && len(snapshot.NetworkInterfaces) > 0 && strings.Join(snapshot.NetworkInterfaces, ",") == strings.Join(previous.NetworkInterfaces, ",") && dt > 0 && dt <= 300 && snapshot.RXBytes != nil && snapshot.TXBytes != nil && previous.RXBytes != nil && previous.TXBytes != nil && *snapshot.RXBytes >= *previous.RXBytes && *snapshot.TXBytes >= *previous.TXBytes {
				rx := float64(*snapshot.RXBytes-*previous.RXBytes) / float64(dt)
				txRate := float64(*snapshot.TXBytes-*previous.TXBytes) / float64(dt)
				snapshot.RXPerSecond = &rx
				snapshot.TXPerSecond = &txRate
			}
		}
		if err = snapshot.Validate(); err != nil {
			return err
		}
	}
	body, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	if err = tx.Exec("INSERT INTO telemetry_samples(node_id,bucket,received_at,body) VALUES(?,?,?,?) ON CONFLICT(node_id,bucket) DO UPDATE SET received_at=excluded.received_at,body=excluded.body", nodeID, strconv.FormatInt(now/60, 10), strconv.FormatInt(now, 10), string(body)); err != nil {
		return err
	}
	if len(rows) > 0 && rows[0]["bucket"] == strconv.FormatInt(now/60, 10) {
		return nil
	}
	if err = tx.Exec("DELETE FROM telemetry_samples WHERE bucket<?", strconv.FormatInt(now/60-119, 10)); err != nil {
		return err
	}
	return tx.Exec("DELETE FROM telemetry_samples WHERE rowid IN (SELECT rowid FROM telemetry_samples ORDER BY bucket DESC LIMIT -1 OFFSET 32768)")
}
func (s *Store) LatestTelemetry(ctx context.Context, nodeID string) (*ObservedTelemetry, error) {
	var result *ObservedTelemetry
	err := s.DB.Read(ctx, func(tx *db.Tx) error {
		rows, err := tx.Query("SELECT received_at,body FROM telemetry_samples WHERE node_id=? ORDER BY bucket DESC LIMIT 1", nodeID)
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		result = &ObservedTelemetry{}
		result.ReceivedAt, err = strconv.ParseInt(rows[0]["received_at"], 10, 64)
		if err != nil {
			return err
		}
		return json.Unmarshal([]byte(rows[0]["body"]), &result.Snapshot)
	})
	return result, err
}
func (s *Store) TelemetryHistory(ctx context.Context, nodeID string) ([]ObservedTelemetry, error) {
	result := []ObservedTelemetry{}
	err := s.DB.Read(ctx, func(tx *db.Tx) error {
		rows, err := tx.Query("SELECT received_at,body FROM telemetry_samples WHERE node_id=? AND bucket>=? ORDER BY bucket LIMIT 120", nodeID, strconv.FormatInt(time.Now().Unix()/60-119, 10))
		if err != nil {
			return err
		}
		for _, row := range rows {
			var item ObservedTelemetry
			item.ReceivedAt, err = strconv.ParseInt(row["received_at"], 10, 64)
			if err != nil {
				return err
			}
			if err = json.Unmarshal([]byte(row["body"]), &item.Snapshot); err != nil {
				return err
			}
			result = append(result, item)
		}
		return nil
	})
	return result, err
}

func (s *Store) FleetTelemetry(ctx context.Context) (map[string]ObservedTelemetry, error) {
	result := map[string]ObservedTelemetry{}
	err := s.DB.Read(ctx, func(tx *db.Tx) error {
		rows, err := tx.Query("SELECT t.node_id,t.received_at,t.body FROM telemetry_samples t JOIN (SELECT node_id,MAX(bucket) AS bucket FROM telemetry_samples GROUP BY node_id) latest ON latest.node_id=t.node_id AND latest.bucket=t.bucket")
		if err != nil {
			return err
		}
		for _, row := range rows {
			var item ObservedTelemetry
			item.ReceivedAt, err = strconv.ParseInt(row["received_at"], 10, 64)
			if err != nil {
				return err
			}
			if err = json.Unmarshal([]byte(row["body"]), &item.Snapshot); err != nil {
				return err
			}
			result[row["node_id"]] = item
		}
		return nil
	})
	return result, err
}
