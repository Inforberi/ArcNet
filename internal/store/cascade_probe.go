package store

import (
	"context"
	"net/netip"
	"strconv"
	"time"

	"github.com/arcvpn/arcvpn/internal/auth"
	"github.com/arcvpn/arcvpn/internal/db"
)

const cascadeProbeSchema = `CREATE TABLE IF NOT EXISTS cascade_probe_grants (
 hash TEXT PRIMARY KEY,deployment_id TEXT NOT NULL UNIQUE REFERENCES cascade_deployments(id) ON DELETE CASCADE,
 version INTEGER NOT NULL,entry_revision INTEGER NOT NULL,entry_digest TEXT NOT NULL,exit_revision INTEGER NOT NULL,exit_digest TEXT NOT NULL,expected_ip TEXT NOT NULL,expires INTEGER NOT NULL)`
const cascadeEvidenceSchema = `CREATE TABLE IF NOT EXISTS cascade_evidence (
 deployment_id TEXT PRIMARY KEY REFERENCES cascade_deployments(id) ON DELETE CASCADE,
 entry_revision INTEGER NOT NULL,entry_digest TEXT NOT NULL,exit_revision INTEGER NOT NULL,exit_digest TEXT NOT NULL,
 exit_ip TEXT NOT NULL,dns_mode TEXT NOT NULL,result TEXT NOT NULL,checked_at INTEGER NOT NULL,expires INTEGER NOT NULL)`
const cascadePublicationSchema = `CREATE TABLE IF NOT EXISTS cascade_publications (
 deployment_id TEXT PRIMARY KEY REFERENCES cascade_deployments(id) ON DELETE CASCADE,
 endpoint_id TEXT NOT NULL REFERENCES cascade_endpoints(id),published_at INTEGER NOT NULL)`

type CascadeProbeGrant struct {
	DeploymentID  string `json:"deployment_id"`
	Version       int64  `json:"version"`
	Token         string `json:"token"`
	ExpiresAt     int64  `json:"expires_at"`
	ExpectedIP    string `json:"expected_exit_ip"`
	EntryRevision int64  `json:"entry_revision"`
	EntryDigest   string `json:"entry_digest"`
	ExitRevision  int64  `json:"exit_revision"`
	ExitDigest    string `json:"exit_digest"`
}
type CascadeProbeReport struct {
	DeploymentID  string `json:"deployment_id"`
	Version       int64  `json:"version"`
	EntryRevision int64  `json:"entry_revision"`
	EntryDigest   string `json:"entry_digest"`
	ExitRevision  int64  `json:"exit_revision"`
	ExitDigest    string `json:"exit_digest"`
	ExitIP        string `json:"exit_ip"`
	DNSMode       string `json:"dns_mode"`
	Result        string `json:"result"`
	DurationMS    int64  `json:"duration_ms"`
}
type CascadeProof struct {
	Result    string `json:"result"`
	ExitIP    string `json:"exit_ip"`
	DNSMode   string `json:"dns_mode"`
	CheckedAt int64  `json:"checked_at"`
	ExpiresAt int64  `json:"expires_at"`
	Current   bool   `json:"current"`
	Published bool   `json:"published"`
}

func cascadeAppliedPair(tx *db.Tx, r db.Row) (db.Row, db.Row, error) {
	views := []db.Row{}
	for _, node := range []string{r["entry_node"], r["exit_node"]} {
		rows, e := tx.Query("SELECT c.*,o.kind FROM node_control c JOIN node_operations o ON o.id=c.deployment_id WHERE c.node_id=?", node)
		if e != nil {
			return nil, nil, e
		}
		if len(rows) != 1 {
			return nil, nil, ErrConflict
		}
		c := rows[0]
		if c["deployment_status"] != "applied" || c["desired_revision"] != c["applied_revision"] || c["kind"] != "apply_cascade_xray_v1" || c["draining"] != "0" {
			return nil, nil, ErrConflict
		}
		views = append(views, c)
	}
	return views[0], views[1], nil
}
func (s *Store) CreateCascadeProbeGrant(ctx context.Context, id string, version int64) (CascadeProbeGrant, error) {
	token, e := auth.Secret(32)
	if e != nil {
		return CascadeProbeGrant{}, e
	}
	var out CascadeProbeGrant
	e = s.DB.WithTx(ctx, func(tx *db.Tx) error {
		r, e := cascadeRow(tx, id)
		if e != nil {
			return e
		}
		if r["version"] != strconv.FormatInt(version, 10) || (r["state"] != "awaiting_probe" && r["state"] != "probe_failed" && r["state"] != "verified" && r["state"] != "published") {
			return ErrConflict
		}
		a, b, e := cascadeAppliedPair(tx, r)
		if e != nil {
			return e
		}
		expected, e := netip.ParseAddr(b["address"])
		if e != nil {
			return ErrConflict
		}
		now := time.Now().Unix()
		expires := now + 300
		out = CascadeProbeGrant{DeploymentID: id, Version: version + 1, Token: token, ExpiresAt: expires, ExpectedIP: expected.Unmap().String()}
		out.EntryRevision, _ = strconv.ParseInt(a["applied_revision"], 10, 64)
		out.ExitRevision, _ = strconv.ParseInt(b["applied_revision"], 10, 64)
		out.EntryDigest = a["desired_digest"]
		out.ExitDigest = b["desired_digest"]
		for _, q := range []string{"DELETE FROM cascade_probe_grants WHERE deployment_id=?", "DELETE FROM cascade_evidence WHERE deployment_id=?", "DELETE FROM cascade_publications WHERE deployment_id=?"} {
			if e := tx.Exec(q, id); e != nil {
				return e
			}
		}
		if e := tx.Exec("UPDATE cascade_deployments SET state='awaiting_probe',version=version+1,updated_at=? WHERE id=?", strconv.FormatInt(now, 10), id); e != nil {
			return e
		}
		if e := tx.Exec("UPDATE node_control SET published=0 WHERE node_id=?", r["entry_node"]); e != nil {
			return e
		}
		return tx.Exec("INSERT INTO cascade_probe_grants(hash,deployment_id,version,entry_revision,entry_digest,exit_revision,exit_digest,expected_ip,expires) VALUES(?,?,?,?,?,?,?,?,?)", auth.HashToken(token), id, strconv.FormatInt(out.Version, 10), a["applied_revision"], out.EntryDigest, b["applied_revision"], out.ExitDigest, out.ExpectedIP, strconv.FormatInt(expires, 10))
	})
	return out, e
}
func (s *Store) CompleteCascadeProbe(ctx context.Context, token string, report CascadeProbeReport) error {
	if len(token) < 40 || len(token) > 128 || report.Version < 1 || report.DurationMS < 0 || report.DurationMS > 90000 || (report.Result != "pass" && report.Result != "fail") {
		return ErrProbeGrant
	}
	return s.DB.WithTx(ctx, func(tx *db.Tx) error {
		rows, e := tx.Query("SELECT * FROM cascade_probe_grants WHERE hash=?", auth.HashToken(token))
		if e != nil {
			return e
		}
		if len(rows) != 1 {
			return ErrProbeGrant
		}
		g := rows[0]
		now := time.Now().Unix()
		expires, _ := strconv.ParseInt(g["expires"], 10, 64)
		if expires < now || g["deployment_id"] != report.DeploymentID || g["version"] != strconv.FormatInt(report.Version, 10) || g["entry_revision"] != strconv.FormatInt(report.EntryRevision, 10) || g["entry_digest"] != report.EntryDigest || g["exit_revision"] != strconv.FormatInt(report.ExitRevision, 10) || g["exit_digest"] != report.ExitDigest {
			return ErrProbeGrant
		}
		r, e := cascadeRow(tx, report.DeploymentID)
		if e != nil {
			return e
		}
		if r["state"] != "awaiting_probe" || r["version"] != g["version"] {
			return ErrConflict
		}
		a, b, e := cascadeAppliedPair(tx, r)
		if e != nil {
			return e
		}
		if a["applied_revision"] != g["entry_revision"] || a["desired_digest"] != g["entry_digest"] || b["applied_revision"] != g["exit_revision"] || b["desired_digest"] != g["exit_digest"] {
			return ErrConflict
		}
		result := "fail"
		ip, e := netip.ParseAddr(report.ExitIP)
		if report.Result == "pass" && e == nil && ip.Unmap().String() == g["expected_ip"] && report.DNSMode == "doh_via_cascade" {
			result = "pass"
		}
		cleanIP := ""
		if e == nil {
			cleanIP = ip.Unmap().String()
		}
		dns := ""
		if report.DNSMode == "doh_via_cascade" {
			dns = report.DNSMode
		}
		if e := tx.Exec("DELETE FROM cascade_probe_grants WHERE hash=?", auth.HashToken(token)); e != nil {
			return e
		}
		if e := tx.Exec("INSERT INTO cascade_evidence(deployment_id,entry_revision,entry_digest,exit_revision,exit_digest,exit_ip,dns_mode,result,checked_at,expires) VALUES(?,?,?,?,?,?,?,?,?,?)", report.DeploymentID, g["entry_revision"], g["entry_digest"], g["exit_revision"], g["exit_digest"], cleanIP, dns, result, strconv.FormatInt(now, 10), strconv.FormatInt(now+1800, 10)); e != nil {
			return e
		}
		state := "probe_failed"
		if result == "pass" {
			state = "verified"
		}
		return tx.Exec("UPDATE cascade_deployments SET state=?,version=version+1,updated_at=? WHERE id=?", state, strconv.FormatInt(now, 10), report.DeploymentID)
	})
}
func cascadeProof(tx *db.Tx, r db.Row) (*CascadeProof, error) {
	rows, e := tx.Query("SELECT e.*,COALESCE(p.endpoint_id,'') AS publication FROM cascade_evidence e LEFT JOIN cascade_publications p ON p.deployment_id=e.deployment_id WHERE e.deployment_id=?", r["id"])
	if e != nil || len(rows) == 0 {
		return nil, e
	}
	g := rows[0]
	checked, _ := strconv.ParseInt(g["checked_at"], 10, 64)
	expires, _ := strconv.ParseInt(g["expires"], 10, 64)
	proof := &CascadeProof{Result: g["result"], ExitIP: g["exit_ip"], DNSMode: g["dns_mode"], CheckedAt: checked, ExpiresAt: expires}
	a, b, e := cascadeAppliedPair(tx, r)
	if e != nil {
		return proof, nil
	}
	proof.Current = g["result"] == "pass" && expires >= time.Now().Unix() && (r["state"] == "verified" || r["state"] == "published") && a["applied_revision"] == g["entry_revision"] && a["desired_digest"] == g["entry_digest"] && b["applied_revision"] == g["exit_revision"] && b["desired_digest"] == g["exit_digest"]
	proof.Published = proof.Current && r["state"] == "published" && g["publication"] != ""
	return proof, nil
}
func (s *Store) PublishCascade(ctx context.Context, id string, version int64) error {
	return s.DB.WithTx(ctx, func(tx *db.Tx) error {
		r, e := cascadeRow(tx, id)
		if e != nil {
			return e
		}
		if r["version"] != strconv.FormatInt(version, 10) || r["state"] != "verified" {
			return ErrConflict
		}
		proof, e := cascadeProof(tx, r)
		if e != nil {
			return e
		}
		if proof == nil || !proof.Current {
			return ErrConflict
		}
		bindings, e := tx.Query("SELECT endpoint_id FROM cascade_bindings WHERE deployment_id=?", id)
		if e != nil {
			return e
		}
		if len(bindings) != 1 {
			return ErrConflict
		}
		now := strconv.FormatInt(time.Now().Unix(), 10)
		if e := tx.Exec("INSERT INTO cascade_publications(deployment_id,endpoint_id,published_at) VALUES(?,?,?)", id, bindings[0]["endpoint_id"], now); e != nil {
			return e
		}
		if e := tx.Exec("UPDATE cascade_deployments SET state='published',version=version+1,updated_at=? WHERE id=?", now, id); e != nil {
			return e
		}
		return tx.Exec("UPDATE node_control SET published=1 WHERE node_id=?", r["entry_node"])
	})
}
func (s *Store) PublishedCascadeEntries(ctx context.Context) ([]string, error) {
	out := []string{}
	e := s.DB.Read(ctx, func(tx *db.Tx) error {
		rows, e := tx.Query("SELECT d.* FROM cascade_deployments d JOIN cascade_publications p ON p.deployment_id=d.id WHERE d.state='published' LIMIT 32")
		if e != nil {
			return e
		}
		for _, r := range rows {
			proof, e := cascadeProof(tx, r)
			if e != nil {
				return e
			}
			if proof != nil && proof.Published {
				out = append(out, r["entry_node"])
			}
		}
		return nil
	})
	return out, e
}
