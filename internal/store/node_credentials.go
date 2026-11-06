package store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/arcvpn/arcvpn/internal/devices"
	"strconv"
	"time"

	"github.com/arcvpn/arcvpn/internal/db"
)

const credentialClockSchema = `CREATE TABLE IF NOT EXISTS credential_clock(id INTEGER PRIMARY KEY CHECK(id=1),version INTEGER NOT NULL CHECK(typeof(version)='integer' AND version BETWEEN 1 AND 9000000000000000))`
const nodeCredentialsSchema = `CREATE TABLE IF NOT EXISTS node_credentials (
 node_id TEXT PRIMARY KEY REFERENCES managed_nodes(id) ON DELETE CASCADE,
 operation_id TEXT NOT NULL,generation INTEGER NOT NULL,applied_generation INTEGER NOT NULL DEFAULT 0)`
const nodeDeviceCapabilitySchema = `CREATE TABLE IF NOT EXISTS node_device_capabilities (
 node_id TEXT PRIMARY KEY REFERENCES managed_nodes(id) ON DELETE CASCADE,protocol INTEGER NOT NULL,checked_at INTEGER NOT NULL)`

var ErrRemoteCredentialsPending = errors.New("remote nodes have not acknowledged current device credentials")

func migrateNodeCredentials(tx *db.Tx) error {
	for _, q := range []string{credentialClockSchema, nodeCredentialsSchema, nodeDeviceCapabilitySchema, "INSERT OR IGNORE INTO credential_clock(id,version) VALUES(1,1)"} {
		if e := tx.Exec(q); e != nil {
			return e
		}
	}
	for _, table := range []string{"users", "user_devices"} {
		for _, event := range []string{"INSERT", "UPDATE", "DELETE"} {
			q := "CREATE TRIGGER IF NOT EXISTS credential_clock_" + table + "_" + event + " AFTER " + event + " ON " + table + " BEGIN UPDATE credential_clock SET version=version+1 WHERE id=1; END"
			if e := tx.Exec(q); e != nil {
				return e
			}
		}
	}
	return nil
}
func credentialGeneration(tx *db.Tx) (int64, error) {
	rows, e := tx.Query("SELECT version FROM credential_clock WHERE id=1")
	if e != nil {
		return 0, e
	}
	if len(rows) != 1 {
		return 0, ErrConflict
	}
	return strconv.ParseInt(rows[0]["version"], 10, 64)
}
func (s *Store) CredentialGeneration(ctx context.Context) (int64, error) {
	var out int64
	e := s.DB.Read(ctx, func(tx *db.Tx) error { var e error; out, e = credentialGeneration(tx); return e })
	return out, e
}
func (s *Store) QueueNodeCredentials(ctx context.Context, node string, config []byte, generation int64, address string, port int) (NodeOperation, error) {
	var out NodeOperation
	e := s.DB.WithTx(ctx, func(tx *db.Tx) error {
		if e := requireNodeWithoutCascade(tx, node); e != nil {
			return e
		}
		current, e := credentialGeneration(tx)
		if e != nil {
			return e
		}
		if current != generation {
			return ErrConflict
		}
		endpoint, e := tx.Query("SELECT address,port FROM node_control WHERE node_id=?", node)
		if e != nil {
			return e
		}
		if len(endpoint) != 1 || endpoint[0]["address"] != address || endpoint[0]["port"] != strconv.Itoa(port) {
			return ErrConflict
		}
		if e := requireDeviceNode(tx, node); e != nil {
			return e
		}
		var compact bytes.Buffer
		if e := json.Compact(&compact, config); e != nil {
			return e
		}
		hash := sha256.Sum256(compact.Bytes())
		digest := hex.EncodeToString(hash[:])
		currentRows, e := tx.Query("SELECT c.*,o.kind FROM node_control c JOIN node_operations o ON o.id=c.deployment_id WHERE c.node_id=?", node)
		if e != nil {
			return e
		}
		if len(currentRows) == 1 {
			r := currentRows[0]
			if r["deployment_status"] == "applied" && r["desired_revision"] == r["applied_revision"] && r["desired_digest"] == digest {
				if e := tx.Exec("INSERT INTO node_credentials(node_id,operation_id,generation,applied_generation) VALUES(?,?,?,?) ON CONFLICT(node_id) DO UPDATE SET operation_id=excluded.operation_id,generation=excluded.generation,applied_generation=excluded.applied_generation", node, r["deployment_id"], strconv.FormatInt(generation, 10), strconv.FormatInt(generation, 10)); e != nil {
					return e
				}
				revision, _ := strconv.ParseInt(r["applied_revision"], 10, 64)
				out = NodeOperation{ID: r["deployment_id"], NodeID: node, Kind: r["kind"], Revision: revision, Digest: digest, State: "applied"}
				return nil
			}
		}
		op, e := s.queueNodeDeployment(tx, node, config, "apply_xray_devices_v1")
		if e != nil {
			return e
		}
		if e := tx.Exec("INSERT INTO node_credentials(node_id,operation_id,generation,applied_generation) VALUES(?,?,?,0) ON CONFLICT(node_id) DO UPDATE SET operation_id=excluded.operation_id,generation=excluded.generation", node, op.ID, strconv.FormatInt(generation, 10)); e != nil {
			return e
		}
		out = op
		return nil
	})
	return out, e
}
func requireDeviceNode(tx *db.Tx, node string) error {
	rows, e := tx.Query("SELECT n.agent_protocol,c.draining,k.protocol,k.checked_at FROM managed_nodes n JOIN node_control c ON c.node_id=n.id JOIN node_device_capabilities k ON k.node_id=n.id JOIN node_profiles p ON p.node_id=n.id WHERE n.id=?", node)
	if e != nil {
		return e
	}
	if len(rows) != 1 {
		return ErrUsageUnsupported
	}
	r := rows[0]
	checked, _ := strconv.ParseInt(r["checked_at"], 10, 64)
	if r["agent_protocol"] != "1" || r["protocol"] != "1" || r["draining"] != "0" || time.Now().Unix()-checked > 180 {
		return ErrUsageUnsupported
	}
	return nil
}
func checkDeviceCoverage(tx *db.Tx) error {
	unsupported, e := tx.Query("SELECT kind FROM protocols WHERE enabled=1 AND kind NOT IN('vless-xhttp','hysteria2','wireguard') LIMIT 1")
	if e != nil {
		return e
	}
	if len(unsupported) > 0 {
		return ErrUsageUnsupported
	}
	nodes, e := tx.Query("SELECT id FROM managed_nodes")
	if e != nil {
		return e
	}
	if len(nodes) > 0 {
		packages, e := readPackages(tx)
		if e != nil {
			return e
		}
		for _, p := range packages {
			if p.QuotaBytes > 0 || p.ExpiresAt > 0 {
				return ErrUsageUnsupported
			}
		}
	}
	for _, node := range nodes {
		if e := requireNodeWithoutCascade(tx, node["id"]); e != nil {
			return e
		}
		if e := requireDeviceNode(tx, node["id"]); e != nil {
			return e
		}
	}
	return nil
}
func (s *Store) CheckDeviceCoverage(ctx context.Context) error {
	return s.DB.Read(ctx, checkDeviceCoverage)
}
func (s *Store) NodeCredentialGeneration(ctx context.Context, node string) (int64, int64, string, error) {
	var desired, applied int64
	var operation string
	e := s.DB.Read(ctx, func(tx *db.Tx) error {
		rows, e := tx.Query("SELECT generation,applied_generation,operation_id FROM node_credentials WHERE node_id=?", node)
		if e != nil {
			return e
		}
		if len(rows) > 0 {
			desired, _ = strconv.ParseInt(rows[0]["generation"], 10, 64)
			applied, _ = strconv.ParseInt(rows[0]["applied_generation"], 10, 64)
			operation = rows[0]["operation_id"]
		}
		return nil
	})
	return desired, applied, operation, e
}
func (s *Store) ReadyDeviceNodes(ctx context.Context) ([]string, error) {
	out := []string{}
	e := s.DB.Read(ctx, func(tx *db.Tx) error {
		if e := checkDeviceCoverage(tx); e != nil {
			return e
		}
		generation, e := credentialGeneration(tx)
		if e != nil {
			return e
		}
		nodes, e := tx.Query("SELECT n.id,c.desired_revision,c.applied_revision,c.deployment_status,r.applied_generation,r.operation_id,c.deployment_id FROM managed_nodes n JOIN node_control c ON c.node_id=n.id LEFT JOIN node_credentials r ON r.node_id=n.id ORDER BY n.id")
		if e != nil {
			return e
		}
		for _, r := range nodes {
			if r["applied_generation"] != strconv.FormatInt(generation, 10) || r["deployment_status"] != "applied" || r["desired_revision"] != r["applied_revision"] || r["operation_id"] != r["deployment_id"] {
				return ErrRemoteCredentialsPending
			}
			out = append(out, r["id"])
		}
		return nil
	})
	return out, e
}
func (s *Store) AuthenticateNode(ctx context.Context, node, secret string) error {
	return s.DB.Read(ctx, func(tx *db.Tx) error {
		ok, e := s.nodeForSecret(tx, secret, node)
		if e != nil {
			return e
		}
		if !ok {
			return ErrNodeUnauthorized
		}
		return nil
	})
}
func (s *Store) HasFinitePackages(ctx context.Context) (bool, error) {
	var yes bool
	e := s.DB.Read(ctx, func(tx *db.Tx) error {
		items, e := readPackages(tx)
		if e != nil {
			return e
		}
		for _, p := range items {
			if p.QuotaBytes > 0 || p.ExpiresAt > 0 {
				yes = true
				break
			}
		}
		return nil
	})
	return yes, e
}

func (s *Store) RevokeDevice(ctx context.Context, user, version int64, device string) error {
	return s.DB.WithTx(ctx, func(tx *db.Tx) error {
		if e := requireNoCascadeLeases(tx); e != nil {
			return e
		}
		rows, e := tx.Query("SELECT body FROM user_devices WHERE user_id=?", strconv.FormatInt(user, 10))
		if e != nil {
			return e
		}
		if len(rows) != 1 {
			return ErrNotFound
		}
		var record devices.Record
		if json.Unmarshal([]byte(rows[0]["body"]), &record) != nil {
			return ErrConflict
		}
		if record.Version != version {
			return ErrConflict
		}
		for i := range record.Devices {
			if record.Devices[i].ID == device {
				if record.Devices[i].RevokedAt == 0 {
					record.Devices[i].RevokedAt = time.Now().Unix()
				}
				record.Version++
				return saveDevices(tx, record)
			}
		}
		return ErrNotFound
	})
}
