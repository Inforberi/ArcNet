package store

import (
	"context"
	"errors"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/arcvpn/arcvpn/internal/auth"
	"github.com/arcvpn/arcvpn/internal/db"
	"github.com/arcvpn/arcvpn/internal/nodeprotocol"
)

var ErrEnrollmentExpired = errors.New("enrollment token invalid or expired")

type Node struct {
	ID            string      `json:"id"`
	Name          string      `json:"name"`
	LastSeen      int64       `json:"last_seen"`
	CreatedAt     int64       `json:"created_at"`
	Version       string      `json:"version"`
	AgentProtocol int         `json:"agent_protocol"`
	XrayState     string      `json:"xray_state"`
	MemoryTotal   uint64      `json:"memory_total"`
	MemoryFree    uint64      `json:"memory_available"`
	Load1         float64     `json:"load_1"`
	Uptime        int64       `json:"uptime_seconds"`
	Online        bool        `json:"online"`
	Control       NodeControl `json:"control"`
}

// CreateEnrollment stores only a hash; a grant is single-use and expires in 10 minutes.
func (s *Store) CreateEnrollment(ctx context.Context, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 64 || strings.ContainsAny(name, "\x00\r\n") {
		return "", errors.New("invalid node name")
	}
	token, err := auth.Secret(32)
	if err != nil {
		return "", err
	}
	now := time.Now().Unix()
	err = s.DB.WithTx(ctx, func(tx *db.Tx) error {
		if err := tx.Exec("DELETE FROM node_enrollments WHERE expires < ?", strconv.FormatInt(now, 10)); err != nil {
			return err
		}
		return tx.Exec("INSERT INTO node_enrollments(hash,name,expires) VALUES(?,?,?)", auth.HashToken(token), name, strconv.FormatInt(now+600, 10))
	})
	return token, err
}

// CreateEnrollmentForNode stores a public IP and a VPN listener port for the
// direct remote node. Public addressing is checked separately from SSH policy.
func (s *Store) CreateEnrollmentForNode(ctx context.Context, name, address string, port int) (string, error) {
	token, _, err := s.CreateEnrollmentForNodeWithID(ctx, name, address, port)
	return token, err
}

// A preallocated node identity allows the controller to associate a completed
// SSH install with its agent even if several nodes have identical display names.
func (s *Store) CreateEnrollmentForNodeWithID(ctx context.Context, name, address string, port int) (string, string, error) {
	ip, err := netip.ParseAddr(address)
	if err != nil || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.Is4In6() || port < 1 || port > 65535 {
		return "", "", errors.New("invalid public node address or port")
	}
	nodeID, err := auth.UUID()
	if err != nil {
		return "", "", err
	}
	token, err := s.CreateEnrollment(ctx, name)
	if err != nil {
		return "", "", err
	}
	if _, err := s.enrollmentDetails(ctx, token, nodeID, address, port); err != nil {
		_ = s.NodeRevokeEnrollment(ctx, token)
		return "", "", err
	}
	return token, nodeID, nil
}

// RegisterNode consumes enrollment and persists a hash of an independently generated agent secret.
func (s *Store) RegisterNode(ctx context.Context, enrollmentToken string) (Node, string, error) {
	if len(enrollmentToken) < 40 || len(enrollmentToken) > 128 {
		return Node{}, "", ErrEnrollmentExpired
	}
	secret, err := auth.Secret(32)
	if err != nil {
		return Node{}, "", err
	}
	id, err := auth.UUID()
	if err != nil {
		return Node{}, "", err
	}
	now := time.Now().Unix()
	var node Node
	err = s.DB.WithTx(ctx, func(tx *db.Tx) error {
		rows, e := tx.Query("SELECT name,expires FROM node_enrollments WHERE hash=?", auth.HashToken(enrollmentToken))
		if e != nil {
			return e
		}
		if len(rows) != 1 {
			return ErrEnrollmentExpired
		}
		until, e := strconv.ParseInt(rows[0]["expires"], 10, 64)
		if e != nil || until < now {
			return ErrEnrollmentExpired
		}
		node = Node{ID: id, Name: rows[0]["name"], CreatedAt: now}
		meta, e := tx.Query("SELECT address,port,node_id FROM node_enrollment_meta WHERE hash=?", auth.HashToken(enrollmentToken))
		if e != nil {
			return e
		}
		address, port := "", "443"
		if len(meta) > 0 {
			address = meta[0]["address"]
			port = meta[0]["port"]
			if meta[0]["node_id"] != "" {
				id = meta[0]["node_id"]
			}
		}
		node.ID = id
		if e = tx.Exec("DELETE FROM node_enrollments WHERE hash=?", auth.HashToken(enrollmentToken)); e != nil {
			return e
		}
		if e = tx.Exec("INSERT INTO managed_nodes(id,name,token_hash,created_at,last_seen,xray_state) VALUES(?,?,?,?,0,'unknown')", id, node.Name, auth.HashToken(secret), strconv.FormatInt(now, 10)); e != nil {
			return e
		}
		return tx.Exec("INSERT INTO node_control(node_id,address,port) VALUES(?,?,?)", id, address, port)
	})
	if err != nil {
		return Node{}, "", err
	}
	return node, secret, nil
}

type NodeHeartbeat struct {
	CascadeProtocol int     `json:"cascade_protocol,omitempty"`
	Version         string  `json:"version"`
	ProtocolVersion int     `json:"protocol_version"`
	XrayState       string  `json:"xray_state"`
	MemoryTotal     uint64  `json:"memory_total"`
	MemoryAvailable uint64  `json:"memory_available"`
	Load1           float64 `json:"load_1"`
	UptimeSeconds   int64   `json:"uptime_seconds"`
}

func (s *Store) Heartbeat(ctx context.Context, bearer string, h NodeHeartbeat) error {
	if len(bearer) < 40 || len(bearer) > 128 {
		return ErrNotFound
	}
	if h.CascadeProtocol < 0 || h.CascadeProtocol > 1 || h.ProtocolVersion < 0 || h.ProtocolVersion > nodeprotocol.Current || len(h.Version) > 100 || len(h.XrayState) > 32 || h.Load1 < 0 || h.Load1 > 10000 || h.MemoryAvailable > h.MemoryTotal || h.UptimeSeconds < 0 {
		return errors.New("invalid heartbeat")
	}
	if h.XrayState != "active" && h.XrayState != "inactive" && h.XrayState != "unknown" {
		return errors.New("invalid xray state")
	}
	return s.DB.WithTx(ctx, func(tx *db.Tx) error {
		rows, err := tx.Query("SELECT id FROM managed_nodes WHERE token_hash=?", auth.HashToken(bearer))
		if err != nil {
			return err
		}
		if len(rows) != 1 {
			return ErrNotFound
		}
		if e := tx.Exec("INSERT INTO cascade_capabilities(node_id,protocol,checked_at) VALUES(?,?,?) ON CONFLICT(node_id) DO UPDATE SET protocol=excluded.protocol,checked_at=excluded.checked_at", rows[0]["id"], strconv.Itoa(h.CascadeProtocol), strconv.FormatInt(time.Now().Unix(), 10)); e != nil {
			return e
		}
		return tx.Exec("UPDATE managed_nodes SET last_seen=?,version=?,agent_protocol=?,xray_state=?,memory_total=?,memory_available=?,load_1=?,uptime_seconds=? WHERE id=?",
			strconv.FormatInt(time.Now().Unix(), 10), h.Version, strconv.Itoa(h.ProtocolVersion), h.XrayState, strconv.FormatUint(h.MemoryTotal, 10), strconv.FormatUint(h.MemoryAvailable, 10), strconv.FormatFloat(h.Load1, 'f', 4, 64), strconv.FormatInt(h.UptimeSeconds, 10), rows[0]["id"])
	})
}
func (s *Store) ListNodes(ctx context.Context) ([]Node, error) {
	out := make([]Node, 0)
	err := s.DB.Read(ctx, func(tx *db.Tx) error {
		rows, e := tx.Query("SELECT n.id,n.name,n.created_at,n.last_seen,n.version,n.agent_protocol,n.xray_state,n.memory_total,n.memory_available,n.load_1,n.uptime_seconds,c.* FROM managed_nodes n LEFT JOIN node_control c ON c.node_id=n.id ORDER BY n.created_at ASC")
		if e != nil {
			return e
		}
		now := time.Now().Unix()
		for _, r := range rows {
			created, _ := strconv.ParseInt(r["created_at"], 10, 64)
			seen, _ := strconv.ParseInt(r["last_seen"], 10, 64)
			total, _ := strconv.ParseUint(r["memory_total"], 10, 64)
			free, _ := strconv.ParseUint(r["memory_available"], 10, 64)
			load, _ := strconv.ParseFloat(r["load_1"], 64)
			up, _ := strconv.ParseInt(r["uptime_seconds"], 10, 64)
			proto, _ := strconv.Atoi(r["agent_protocol"])
			out = append(out, Node{AgentProtocol: proto, ID: r["id"], Name: r["name"], CreatedAt: created, LastSeen: seen, Version: r["version"], XrayState: r["xray_state"], MemoryTotal: total, MemoryFree: free, Load1: load, Uptime: up, Online: seen > 0 && now-seen <= 75, Control: nodeControlFromRow(r)})
		}
		return nil
	})
	return out, err
}
func (s *Store) RemoveNode(ctx context.Context, id string) error {
	return s.DB.WithTx(ctx, func(tx *db.Tx) error {
		rows, err := tx.Query("SELECT id FROM managed_nodes WHERE id=?", id)
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			return ErrNotFound
		}
		if err := requireNodeWithoutRoutes(tx, id); err != nil {
			return err
		}
		if err := tx.Exec("DELETE FROM telemetry_samples WHERE node_id=?", id); err != nil {
			return err
		}
		return tx.Exec("DELETE FROM managed_nodes WHERE id=?", id)
	})
}

// EnrollmentValid authorizes downloading the signed-transport node binary.
// It does not consume the token; registration consumes it atomically.
func (s *Store) EnrollmentValid(ctx context.Context, token string) (bool, error) {
	if len(token) < 40 || len(token) > 128 {
		return false, nil
	}
	valid := false
	err := s.DB.Read(ctx, func(tx *db.Tx) error {
		rows, e := tx.Query("SELECT expires FROM node_enrollments WHERE hash=?", auth.HashToken(token))
		if e != nil {
			return e
		}
		if len(rows) != 1 {
			return nil
		}
		until, e := strconv.ParseInt(rows[0]["expires"], 10, 64)
		valid = e == nil && until >= time.Now().Unix()
		return nil
	})
	return valid, err
}
