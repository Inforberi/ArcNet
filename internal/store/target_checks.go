package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"sort"
	"strconv"
	"time"

	"github.com/arcvpn/arcvpn/internal/auth"
	"github.com/arcvpn/arcvpn/internal/db"
	"github.com/arcvpn/arcvpn/internal/targetcheck"
)

const targetChecksSchema = `CREATE TABLE IF NOT EXISTS target_checks (id TEXT PRIMARY KEY,node_id TEXT NOT NULL,candidates TEXT NOT NULL,digest TEXT NOT NULL,state TEXT NOT NULL,created_at INTEGER NOT NULL,expires_at INTEGER NOT NULL,lease_until INTEGER NOT NULL DEFAULT 0,attempts INTEGER NOT NULL DEFAULT 0,result TEXT NOT NULL DEFAULT '')`
const targetCapabilitySchema = `CREATE TABLE IF NOT EXISTS target_capabilities (node_id TEXT PRIMARY KEY REFERENCES managed_nodes(id) ON DELETE CASCADE,last_seen INTEGER NOT NULL)`

type TargetJob struct {
	Version    int                     `json:"version"`
	ID         string                  `json:"id"`
	NodeID     string                  `json:"node_id"`
	Candidates []targetcheck.Candidate `json:"candidates"`
	Digest     string                  `json:"digest"`
	State      string                  `json:"state"`
	CreatedAt  int64                   `json:"created_at"`
	ExpiresAt  int64                   `json:"expires_at"`
	Report     *targetcheck.Report     `json:"report,omitempty"`
}
type TargetNode struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Supported bool   `json:"supported"`
	Online    bool   `json:"online"`
}

func targetJob(row db.Row) (TargetJob, error) {
	var job TargetJob
	job.Version = 1
	job.ID = row["id"]
	job.NodeID = row["node_id"]
	job.State = row["state"]
	job.Digest = row["digest"]
	job.CreatedAt, _ = strconv.ParseInt(row["created_at"], 10, 64)
	job.ExpiresAt, _ = strconv.ParseInt(row["expires_at"], 10, 64)
	if err := json.Unmarshal([]byte(row["candidates"]), &job.Candidates); err != nil {
		return job, err
	}
	if row["result"] != "" {
		if err := json.Unmarshal([]byte(row["result"]), &job.Report); err != nil {
			return job, err
		}
	}
	if time.Now().Unix() > job.ExpiresAt && job.State != "done" {
		job.State = "expired"
	}
	return job, nil
}
func (s *Store) TargetNodes(ctx context.Context) ([]TargetNode, error) {
	result := []TargetNode{{ID: "local", Name: "Сервер панели", Supported: true, Online: true}}
	err := s.DB.Read(ctx, func(tx *db.Tx) error {
		rows, e := tx.Query("SELECT n.id,n.name,n.last_seen,c.last_seen AS capability_seen FROM managed_nodes n LEFT JOIN target_capabilities c ON c.node_id=n.id ORDER BY n.created_at")
		if e != nil {
			return e
		}
		for _, row := range rows {
			seen, _ := strconv.ParseInt(row["last_seen"], 10, 64)
			capSeen, _ := strconv.ParseInt(row["capability_seen"], 10, 64)
			now := time.Now().Unix()
			result = append(result, TargetNode{ID: row["id"], Name: row["name"], Online: seen > 0 && now-seen <= 75, Supported: capSeen > 0 && now-capSeen <= 150})
		}
		return nil
	})
	return result, err
}
func (s *Store) TargetHello(ctx context.Context, nodeID, secret string) error {
	return s.DB.WithTx(ctx, func(tx *db.Tx) error {
		ok, e := s.nodeForSecret(tx, secret, nodeID)
		if e != nil {
			return e
		}
		if !ok {
			return ErrNodeUnauthorized
		}
		return tx.Exec("INSERT INTO target_capabilities(node_id,last_seen) VALUES(?,?) ON CONFLICT(node_id) DO UPDATE SET last_seen=excluded.last_seen", nodeID, strconv.FormatInt(time.Now().Unix(), 10))
	})
}
func (s *Store) QueueTargetCheck(ctx context.Context, nodeID string, candidates []targetcheck.Candidate) (TargetJob, error) {
	normalized, err := targetcheck.Normalize(candidates)
	if err != nil {
		return TargetJob{}, err
	}
	id, err := auth.UUID()
	if err != nil {
		return TargetJob{}, err
	}
	body, _ := json.Marshal(normalized)
	digest := sha256.Sum256(body)
	now := time.Now().Unix()
	job := TargetJob{Version: 1, ID: id, NodeID: nodeID, Candidates: normalized, Digest: hex.EncodeToString(digest[:]), State: "queued", CreatedAt: now, ExpiresAt: now + 120}
	err = s.DB.WithTx(ctx, func(tx *db.Tx) error {
		if nodeID != "local" {
			rows, e := tx.Query("SELECT n.id FROM managed_nodes n JOIN target_capabilities c ON c.node_id=n.id WHERE n.id=? AND n.last_seen>=? AND c.last_seen>=?", nodeID, strconv.FormatInt(now-75, 10), strconv.FormatInt(now-150, 10))
			if e != nil {
				return e
			}
			if len(rows) != 1 {
				return ErrIncompatibleNodeProtocol
			}
		}
		if e := tx.Exec("DELETE FROM target_checks WHERE created_at<?", strconv.FormatInt(now-86400, 10)); e != nil {
			return e
		}
		rows, e := tx.Query("SELECT id FROM target_checks WHERE node_id=? AND state IN ('queued','running') AND expires_at>=?", nodeID, strconv.FormatInt(now, 10))
		if e != nil {
			return e
		}
		if len(rows) > 0 {
			return ErrNodeBusy
		}
		count, e := tx.Query("SELECT COUNT(*) AS count FROM target_checks")
		if e != nil {
			return e
		}
		n, _ := strconv.Atoi(count[0]["count"])
		if n >= 512 {
			return ErrNodeBusy
		}
		return tx.Exec("INSERT INTO target_checks(id,node_id,candidates,digest,state,created_at,expires_at) VALUES(?,?,?,?,'queued',?,?)", job.ID, nodeID, string(body), job.Digest, strconv.FormatInt(now, 10), strconv.FormatInt(job.ExpiresAt, 10))
	})
	return job, err
}
func (s *Store) TargetCheck(ctx context.Context, id string) (TargetJob, error) {
	var job TargetJob
	err := s.DB.Read(ctx, func(tx *db.Tx) error {
		rows, e := tx.Query("SELECT * FROM target_checks WHERE id=?", id)
		if e != nil {
			return e
		}
		if len(rows) != 1 {
			return ErrNotFound
		}
		job, e = targetJob(rows[0])
		return e
	})
	return job, err
}
func (s *Store) PollTargetCheck(ctx context.Context, nodeID, secret string) (*TargetJob, error) {
	var result *TargetJob
	err := s.DB.WithTx(ctx, func(tx *db.Tx) error {
		ok, e := s.nodeForSecret(tx, secret, nodeID)
		if e != nil {
			return e
		}
		if !ok {
			return ErrNodeUnauthorized
		}
		now := time.Now().Unix()
		rows, e := tx.Query("SELECT * FROM target_checks WHERE node_id=? AND state IN ('queued','running') AND expires_at>=? AND lease_until<? AND attempts<2 ORDER BY created_at LIMIT 1", nodeID, strconv.FormatInt(now, 10), strconv.FormatInt(now, 10))
		if e != nil {
			return e
		}
		if len(rows) == 0 {
			return nil
		}
		job, e := targetJob(rows[0])
		if e != nil {
			return e
		}
		job.State = "running"
		result = &job
		return tx.Exec("UPDATE target_checks SET state='running',lease_until=?,attempts=attempts+1 WHERE id=?", strconv.FormatInt(now+45, 10), job.ID)
	})
	return result, err
}
func validateTargetReport(job TargetJob, report targetcheck.Report) (targetcheck.Report, error) {
	if len(report.Results) != len(job.Candidates) {
		return report, ErrConflict
	}
	report.Recommended = nil
	report.MeasuredFrom = job.NodeID
	report.VPNVerified = false
	report.CheckedAt = time.Now().Unix()
	report.ExpiresAt = report.CheckedAt + targetcheck.ResultTTL
	best := math.Inf(1)
	for i := range report.Results {
		result := &report.Results[i]
		if result.Candidate != job.Candidates[i] || len(result.Samples) > targetcheck.Samples || len(result.Error) > 200 {
			return report, ErrConflict
		}
		values := []float64{}
		for _, sample := range result.Samples {
			duration := sample.TCPMS + sample.TLSMS
			if math.IsNaN(duration) || math.IsInf(duration, 0) || sample.TCPMS < 0 || sample.TLSMS < 0 || duration > 3500 {
				return report, ErrConflict
			}
			values = append(values, duration)
		}
		sort.Float64s(values)
		result.VPNVerified = false
		result.CheckedAt = report.CheckedAt
		result.Successes = len(values)
		result.Eligible = len(values) == targetcheck.Samples && result.HTTPVerified && result.HTTPStatus >= 200 && result.HTTPStatus < 300 && result.Error == ""
		result.MedianMS = 0
		result.P95MS = 0
		if len(values) > 0 {
			result.MedianMS = values[len(values)/2]
			result.P95MS = values[len(values)-1]
		}
		if result.Eligible && result.MedianMS < best {
			candidate := result.Candidate
			report.Recommended = &candidate
			best = result.MedianMS
		}
	}
	return report, nil
}
func (s *Store) CompleteTargetCheck(ctx context.Context, id, nodeID, secret, digest string, report targetcheck.Report) error {
	return s.DB.WithTx(ctx, func(tx *db.Tx) error {
		if nodeID != "local" {
			ok, e := s.nodeForSecret(tx, secret, nodeID)
			if e != nil {
				return e
			}
			if !ok {
				return ErrNodeUnauthorized
			}
		} else if secret != "" {
			return ErrNodeUnauthorized
		}
		rows, e := tx.Query("SELECT * FROM target_checks WHERE id=? AND node_id=? AND digest=?", id, nodeID, digest)
		if e != nil {
			return e
		}
		if len(rows) != 1 {
			return ErrNotFound
		}
		job, e := targetJob(rows[0])
		if e != nil {
			return e
		}
		if job.State == "done" {
			return nil
		} // immutable first report; lost ACK is idempotent
		if job.State == "expired" || (nodeID != "local" && job.State != "running") {
			return ErrConflict
		}
		report, e = validateTargetReport(job, report)
		if e != nil {
			return e
		}
		body, e := json.Marshal(report)
		if e != nil {
			return e
		}
		return tx.Exec("UPDATE target_checks SET state='done',result=? WHERE id=?", string(body), id)
	})
}
func (s *Store) TargetCatalog(ctx context.Context) (targetcheck.Catalog, error) {
	c := targetcheck.DefaultCatalog()
	err := s.DB.Read(ctx, func(tx *db.Tx) error {
		rows, e := tx.Query("SELECT value FROM settings WHERE key='target_catalog'")
		if e != nil {
			return e
		}
		if len(rows) == 0 {
			return nil
		}
		return json.Unmarshal([]byte(rows[0]["value"]), &c)
	})
	if err != nil {
		return c, err
	}
	return targetcheck.ValidateCatalog(c)
}
func (s *Store) SaveTargetCatalog(ctx context.Context, c targetcheck.Catalog, expected int64) error {
	c, e := targetcheck.ValidateCatalog(c)
	if e != nil {
		return e
	}
	return s.DB.WithTx(ctx, func(tx *db.Tx) error {
		rows, e := tx.Query("SELECT value FROM settings WHERE key='target_catalog'")
		if e != nil {
			return e
		}
		old := targetcheck.DefaultCatalog()
		if len(rows) > 0 {
			if e = json.Unmarshal([]byte(rows[0]["value"]), &old); e != nil {
				return e
			}
		}
		if old.Version != expected {
			return ErrConflict
		}
		if old.Version >= math.MaxInt64-1 {
			return ErrConflict
		}
		if c.FeedVersion > 0 {
			if c.ExpiresAt <= time.Now().Unix() {
				return ErrConflict
			}
			watermark, err := tx.Query("SELECT value FROM settings WHERE key='target_catalog_feed_version'")
			if err != nil {
				return err
			}
			var previous int64
			if len(watermark) > 0 {
				previous, err = strconv.ParseInt(watermark[0]["value"], 10, 64)
				if err != nil {
					return err
				}
			}
			if c.FeedVersion <= previous {
				return ErrConflict
			}
			if err = tx.Exec("INSERT INTO settings(key,value) VALUES('target_catalog_feed_version',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", strconv.FormatInt(c.FeedVersion, 10)); err != nil {
				return err
			}
		}
		c.Version = old.Version + 1
		body, e := json.Marshal(c)
		if e != nil {
			return e
		}
		return tx.Exec("INSERT INTO settings(key,value) VALUES('target_catalog',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", string(body))
	})
}
