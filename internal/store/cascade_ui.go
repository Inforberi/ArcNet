package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/arcvpn/arcvpn/internal/auth"
	"github.com/arcvpn/arcvpn/internal/db"
	"github.com/arcvpn/arcvpn/internal/routes"
)

// Endpoint identity survives address/port changes; it currently owns one
// complete managed TCP inbound, never an implied subset of its users.
const cascadeEndpointsSchema = `CREATE TABLE IF NOT EXISTS cascade_endpoints (
 id TEXT PRIMARY KEY,node_id TEXT NOT NULL UNIQUE REFERENCES managed_nodes(id) ON DELETE CASCADE,
 inbound_tag TEXT NOT NULL CHECK(inbound_tag='vless-reality'))`
const cascadeBindingsSchema = `CREATE TABLE IF NOT EXISTS cascade_bindings (
 deployment_id TEXT PRIMARY KEY REFERENCES cascade_deployments(id) ON DELETE CASCADE,
 endpoint_id TEXT NOT NULL REFERENCES cascade_endpoints(id),
 scope TEXT NOT NULL CHECK(scope='all_tcp_users'))`

type CascadeBinding struct {
	EndpointID string `json:"endpoint_id"`
	NodeID     string `json:"node_id"`
	InboundTag string `json:"inbound_tag"`
	Scope      string `json:"scope"`
}
type CascadeStepView struct {
	Step     string `json:"step"`
	NodeID   string `json:"node_id"`
	State    string `json:"state"`
	Revision int64  `json:"revision"`
}
type CascadeView struct {
	Proof               *CascadeProof `json:"proof"`
	ProbeGrantExpiresAt int64         `json:"probe_grant_expires_at"`
	CascadeDeployment
	Binding *CascadeBinding   `json:"binding"`
	Steps   []CascadeStepView `json:"steps"`
}
type CascadePreflight struct {
	RosterDigest string
	Entry, Exit  NodeControl
	UserCount    int
	EndpointID   string
}

func (s *Store) CascadePreflight(ctx context.Context, id string, version int64) (CascadePreflight, error) {
	var out CascadePreflight
	err := s.DB.Read(ctx, func(tx *db.Tx) error {
		rows, e := tx.Query("SELECT body,version FROM route_policies WHERE id=?", id)
		if e != nil {
			return e
		}
		if len(rows) != 1 {
			return ErrNotFound
		}
		if rows[0]["version"] != strconv.FormatInt(version, 10) {
			return ErrConflict
		}
		var p routes.Policy
		if json.Unmarshal([]byte(rows[0]["body"]), &p) != nil || p.Mode != "cascade" || p.FailureMode != "closed" || len(p.BackupExits) != 0 || p.EntryNode == routes.LocalNode || p.PrimaryExit == routes.LocalNode {
			return ErrRouteInvalid
		}
		if e := requireRouteWithoutCascade(tx, id); e != nil {
			return e
		}
		for _, node := range []string{p.EntryNode, p.PrimaryExit} {
			if e := requireNodeWithoutCascade(tx, node); e != nil {
				return e
			}
		}
		a, e := readyCascadeNode(tx, p.EntryNode)
		if e != nil {
			return e
		}
		b, e := readyCascadeNode(tx, p.PrimaryExit)
		if e != nil {
			return e
		}
		out.Entry = nodeControlFromRow(a)
		out.Exit = nodeControlFromRow(b)
		out.RosterDigest, e = cascadeRoster(tx)
		if e != nil {
			return e
		}
		users, e := tx.Query("SELECT id FROM users WHERE enabled=1")
		if e != nil {
			return e
		}
		out.UserCount = len(users)
		endpoints, e := tx.Query("SELECT id FROM cascade_endpoints WHERE node_id=?", p.EntryNode)
		if e != nil {
			return e
		}
		if len(endpoints) > 0 {
			out.EndpointID = endpoints[0]["id"]
		}
		return nil
	})
	return out, err
}
func bindCascadeEndpoint(tx *db.Tx, deployment, node string) error {
	rows, e := tx.Query("SELECT id FROM cascade_endpoints WHERE node_id=?", node)
	if e != nil {
		return e
	}
	var id string
	if len(rows) == 0 {
		id, e = auth.UUID()
		if e != nil {
			return e
		}
		if e := tx.Exec("INSERT INTO cascade_endpoints(id,node_id,inbound_tag) VALUES(?,?,'vless-reality')", id, node); e != nil {
			return e
		}
	} else {
		id = rows[0]["id"]
	}
	return tx.Exec("INSERT INTO cascade_bindings(deployment_id,endpoint_id,scope) VALUES(?,?,'all_tcp_users')", deployment, id)
}
func (s *Store) CascadeDeployments(ctx context.Context) ([]CascadeView, error) {
	out := []CascadeView{}
	err := s.DB.Read(ctx, func(tx *db.Tx) error {
		rows, e := tx.Query("SELECT * FROM cascade_deployments ORDER BY created_at DESC,id LIMIT 32")
		if e != nil {
			return e
		}
		for _, row := range rows {
			item := CascadeView{CascadeDeployment: cascadeProjection(row), Steps: []CascadeStepView{}}
			bindings, e := tx.Query("SELECT b.endpoint_id,b.scope,e.node_id,e.inbound_tag FROM cascade_bindings b JOIN cascade_endpoints e ON e.id=b.endpoint_id WHERE b.deployment_id=?", item.ID)
			if e != nil {
				return e
			}
			if len(bindings) > 0 {
				b := bindings[0]
				item.Binding = &CascadeBinding{EndpointID: b["endpoint_id"], NodeID: b["node_id"], InboundTag: b["inbound_tag"], Scope: b["scope"]}
			}
			steps, e := tx.Query("SELECT s.step,s.node_id,s.revision,COALESCE(o.state,'not_queued') AS state FROM cascade_steps s LEFT JOIN node_operations o ON o.id=s.operation_id WHERE s.deployment_id=? ORDER BY CASE s.step WHEN 'apply_exit' THEN 1 WHEN 'apply_entry' THEN 2 WHEN 'restore_entry' THEN 3 ELSE 4 END", item.ID)
			if e != nil {
				return e
			}
			for _, r := range steps {
				revision, _ := strconv.ParseInt(r["revision"], 10, 64)
				item.Steps = append(item.Steps, CascadeStepView{Step: r["step"], NodeID: r["node_id"], State: r["state"], Revision: revision})
			}
			item.Proof, e = cascadeProof(tx, row)
			if e != nil {
				return e
			}
			grants, e := tx.Query("SELECT expires FROM cascade_probe_grants WHERE deployment_id=?", item.ID)
			if e != nil {
				return e
			}
			if len(grants) > 0 {
				item.ProbeGrantExpiresAt, _ = strconv.ParseInt(grants[0]["expires"], 10, 64)
			}
			out = append(out, item)
		}
		return nil
	})
	return out, err
}

type NodeConfigSnapshot struct {
	Config   []byte `json:"-"`
	Digest   string
	Revision int64
}

func (s *Store) AcknowledgedNodeConfig(ctx context.Context, node string) (NodeConfigSnapshot, error) {
	var out NodeConfigSnapshot
	e := s.DB.Read(ctx, func(tx *db.Tx) error {
		rows, e := tx.Query("SELECT * FROM node_control WHERE node_id=?", node)
		if e != nil {
			return e
		}
		if len(rows) != 1 {
			return ErrNotFound
		}
		r := rows[0]
		if r["deployment_status"] != "applied" || r["desired_revision"] == "0" || r["desired_revision"] != r["applied_revision"] {
			return ErrCascadeNotReady
		}
		plain, e := s.openNodeConfig(r["desired_ciphertext"], fmt.Sprintf("%s:%s:%s", node, r["desired_revision"], r["desired_digest"]))
		if e != nil {
			return e
		}
		digest := sha256.Sum256(plain)
		if hex.EncodeToString(digest[:]) != r["desired_digest"] {
			return ErrCascadeNotReady
		}
		out = NodeConfigSnapshot{Config: plain, Digest: r["desired_digest"]}
		out.Revision, _ = strconv.ParseInt(r["desired_revision"], 10, 64)
		return nil
	})
	return out, e
}
