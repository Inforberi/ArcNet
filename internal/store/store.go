package store

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/arcvpn/arcvpn/internal/db"
	"github.com/arcvpn/arcvpn/internal/routes"
	"github.com/arcvpn/arcvpn/internal/routingprofile"
	"github.com/arcvpn/arcvpn/internal/usage"
)

var ErrNotFound = errors.New("not found")
var ErrConflict = errors.New("conflict")

type Store struct{ DB *db.DB }
type User struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	UUID         string `json:"uuid"`
	Enabled      bool   `json:"enabled"`
	CreatedAt    int64  `json:"created_at"`
	Subscription string `json:"-"`
}
type Reality struct {
	Address string `json:"address"`
	Port    int    `json:"port"`
	Target  string `json:"target"`
	SNI     string `json:"sni"`
	Private string `json:"-"`
	Public  string `json:"public_key"`
	ShortID string `json:"short_id"`
	Enabled bool   `json:"enabled"`
}

func (s *Store) Migrate(ctx context.Context) error {
	err := s.DB.WithTx(ctx, func(tx *db.Tx) error {
		statements := []string{
			`CREATE TABLE IF NOT EXISTS admins (username TEXT PRIMARY KEY, hash TEXT NOT NULL)`,
			`CREATE TABLE IF NOT EXISTS admin_mfa (id INTEGER PRIMARY KEY CHECK(id=1), secret TEXT NOT NULL DEFAULT '', pending_secret TEXT NOT NULL DEFAULT '', pending_until INTEGER NOT NULL DEFAULT 0, last_step INTEGER NOT NULL DEFAULT -1)`,
			`CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
			`CREATE TABLE IF NOT EXISTS sessions (hash TEXT PRIMARY KEY, csrf TEXT NOT NULL, expires INTEGER NOT NULL)`,
			`CREATE TABLE IF NOT EXISTS users (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL UNIQUE, uuid TEXT NOT NULL UNIQUE, token TEXT NOT NULL UNIQUE, enabled INTEGER NOT NULL DEFAULT 1, created_at INTEGER NOT NULL)`,
			`CREATE TABLE IF NOT EXISTS config (id INTEGER PRIMARY KEY CHECK (id=1), address TEXT NOT NULL, port INTEGER NOT NULL, target TEXT NOT NULL, sni TEXT NOT NULL, priv TEXT NOT NULL, pub TEXT NOT NULL, short_id TEXT NOT NULL, enabled INTEGER NOT NULL DEFAULT 0)`,
			`CREATE TABLE IF NOT EXISTS events (id INTEGER PRIMARY KEY AUTOINCREMENT, time INTEGER NOT NULL, action TEXT NOT NULL, details TEXT NOT NULL)`,
			`CREATE INDEX IF NOT EXISTS sessions_expires_idx ON sessions(expires)`,
			`CREATE INDEX IF NOT EXISTS events_time_idx ON events(time)`,
			`CREATE TABLE IF NOT EXISTS node_enrollments(hash TEXT PRIMARY KEY,name TEXT NOT NULL,expires INTEGER NOT NULL)`,
			`CREATE INDEX IF NOT EXISTS node_enrollments_exp ON node_enrollments(expires)`,
			`CREATE TABLE IF NOT EXISTS managed_nodes(id TEXT PRIMARY KEY,name TEXT NOT NULL,token_hash TEXT NOT NULL UNIQUE,created_at INTEGER NOT NULL,last_seen INTEGER NOT NULL DEFAULT 0,version TEXT NOT NULL DEFAULT '',xray_state TEXT NOT NULL DEFAULT 'unknown',memory_total TEXT NOT NULL DEFAULT '0',memory_available TEXT NOT NULL DEFAULT '0',load_1 TEXT NOT NULL DEFAULT '0',uptime_seconds TEXT NOT NULL DEFAULT '0')`,
			packagesSchema, checkpointsSchema, debitsSchema, profilesSchema, credentialsSchema, credentialsAddressIndex, routePolicySchema, routingProfilesSchema, targetChecksSchema, targetCapabilitySchema, telemetrySchema, telemetryTimeIndex,
		}
		for _, q := range statements {
			if err := tx.Exec(q); err != nil {
				return err
			}
		}
		// Older databases predate protocol negotiation. Preserve existing nodes,
		// but require a new compatible heartbeat before sending any operation.
		columns, err := tx.Query("PRAGMA table_info(managed_nodes)")
		if err != nil {
			return err
		}
		found := false
		for _, col := range columns {
			if col["name"] == "agent_protocol" {
				found = true
				break
			}
		}
		if !found {
			if err := tx.Exec("ALTER TABLE managed_nodes ADD COLUMN agent_protocol INTEGER NOT NULL DEFAULT 0"); err != nil {
				return err
			}
		}
		for _, q := range []string{nodeControlSchema, nodeOpSchema, nodeEnrollMetaSchema, nodeProfileSchema, nodeBootstrapSchema, `CREATE INDEX IF NOT EXISTS node_operations_node_idx ON node_operations(node_id,created_at DESC)`, `INSERT OR IGNORE INTO node_control(node_id) SELECT id FROM managed_nodes`} {
			if err := tx.Exec(q); err != nil {
				return err
			}
		}
		if err := migrateNodeControlDrain(tx); err != nil {
			return err
		}
		if err := migrateNodeControlProbe(tx); err != nil {
			return err
		}
		// Node bootstrap journal was introduced before operation-type fields.
		// Migrate in-place so historical install IDs remain available for
		// root-marker confirmation during remote removal.
		opColumns, err := tx.Query("PRAGMA table_info(node_bootstrap_operations)")
		if err != nil {
			return err
		}
		has := map[string]bool{}
		for _, col := range opColumns {
			has[col["name"]] = true
		}
		if !has["operation_type"] {
			if err = tx.Exec("ALTER TABLE node_bootstrap_operations ADD COLUMN operation_type TEXT NOT NULL DEFAULT 'install'"); err != nil {
				return err
			}
		}
		if !has["removal_mode"] {
			if err = tx.Exec("ALTER TABLE node_bootstrap_operations ADD COLUMN removal_mode TEXT NOT NULL DEFAULT ''"); err != nil {
				return err
			}
		}
		if !has["target_version"] {
			if err = tx.Exec("ALTER TABLE node_bootstrap_operations ADD COLUMN target_version TEXT NOT NULL DEFAULT ''"); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	return s.initializeNodeKey(ctx)
}
func (s *Store) HasAdmin(ctx context.Context) (bool, error) {
	var yes bool
	err := s.DB.Read(ctx, func(tx *db.Tx) error {
		rows, err := tx.Query("SELECT username FROM admins LIMIT 1")
		yes = len(rows) > 0
		return err
	})
	return yes, err
}
func (s *Store) CreateAdmin(ctx context.Context, username, hash string) error {
	return s.DB.WithTx(ctx, func(tx *db.Tx) error {
		r, e := tx.Query("SELECT username FROM admins LIMIT 1")
		if e != nil {
			return e
		}
		if len(r) > 0 {
			return ErrConflict
		}
		return tx.Exec("INSERT INTO admins(username,hash) VALUES(?,?)", username, hash)
	})
}

// AdminUsername returns the administrator configured during initial setup.
// The current standalone profile supports one administrator.
func (s *Store) AdminUsername(ctx context.Context) (string, error) {
	var name string
	err := s.DB.Read(ctx, func(tx *db.Tx) error {
		rows, err := tx.Query("SELECT username FROM admins LIMIT 1")
		if err != nil {
			return err
		}
		if len(rows) > 0 {
			name = rows[0]["username"]
		}
		return nil
	})
	return name, err
}

func (s *Store) AdminHash(ctx context.Context, username string) (string, error) {
	var result string
	err := s.DB.Read(ctx, func(tx *db.Tx) error {
		r, e := tx.Query("SELECT hash FROM admins WHERE username=?", username)
		if e != nil {
			return e
		}
		if len(r) > 0 {
			result = r[0]["hash"]
		}
		return nil
	})
	return result, err
}
func (s *Store) AddSession(ctx context.Context, hash, csrf string, until int64) error {
	return s.DB.WithTx(ctx, func(tx *db.Tx) error {
		if err := tx.Exec("DELETE FROM sessions WHERE expires < ?", strconv.FormatInt(time.Now().Unix(), 10)); err != nil {
			return err
		}
		return tx.Exec("INSERT INTO sessions(hash,csrf,expires) VALUES(?,?,?)", hash, csrf, strconv.FormatInt(until, 10))
	})
}
func (s *Store) Session(ctx context.Context, hash string) (string, bool, error) {
	var token string
	var valid bool
	err := s.DB.Read(ctx, func(tx *db.Tx) error {
		r, e := tx.Query("SELECT csrf,expires FROM sessions WHERE hash=?", hash)
		if e != nil {
			return e
		}
		if len(r) == 0 {
			return nil
		}
		n, _ := strconv.ParseInt(r[0]["expires"], 10, 64)
		valid = n > time.Now().Unix()
		if valid {
			token = r[0]["csrf"]
		}
		return nil
	})
	return token, valid, err
}
func (s *Store) DropSession(ctx context.Context, hash string) error {
	return s.DB.WithTx(ctx, func(tx *db.Tx) error { return tx.Exec("DELETE FROM sessions WHERE hash=?", hash) })
}
func userFromRow(r db.Row) User {
	id, _ := strconv.ParseInt(r["id"], 10, 64)
	created, _ := strconv.ParseInt(r["created_at"], 10, 64)
	return User{ID: id, Name: r["name"], UUID: r["uuid"], Enabled: r["enabled"] == "1", CreatedAt: created, Subscription: r["token"]}
}

const userQuery = "SELECT id,name,uuid,token,enabled,created_at FROM users"

func (s *Store) ListUsers(ctx context.Context) ([]User, error) {
	var out []User
	err := s.DB.Read(ctx, func(tx *db.Tx) error {
		rows, err := tx.Query(userQuery + " ORDER BY id DESC")
		if err != nil {
			return err
		}
		out = make([]User, 0, len(rows))
		for _, r := range rows {
			out = append(out, userFromRow(r))
		}
		return nil
	})
	return out, err
}
func (s *Store) ByToken(ctx context.Context, token string) (User, error) {
	var user User
	err := s.DB.Read(ctx, func(tx *db.Tx) error {
		r, e := tx.Query(userQuery+" WHERE token=?", token)
		if e != nil {
			return e
		}
		if len(r) == 0 {
			return ErrNotFound
		}
		user = userFromRow(r[0])
		return nil
	})
	return user, err
}
func (s *Store) ByID(ctx context.Context, id int64) (User, error) {
	var user User
	err := s.DB.Read(ctx, func(tx *db.Tx) error {
		r, e := tx.Query(userQuery+" WHERE id=?", strconv.FormatInt(id, 10))
		if e != nil {
			return e
		}
		if len(r) == 0 {
			return ErrNotFound
		}
		user = userFromRow(r[0])
		return nil
	})
	return user, err
}
func (s *Store) InsertUser(ctx context.Context, u User) error {
	return s.DB.WithTx(ctx, func(tx *db.Tx) error {
		if err := tx.Exec("INSERT INTO users(name,uuid,token,enabled,created_at) VALUES(?,?,?,?,?)", u.Name, u.UUID, u.Subscription, boolean(u.Enabled), strconv.FormatInt(u.CreatedAt, 10)); err != nil {
			if strings.Contains(err.Error(), "UNIQUE") {
				return ErrConflict
			}
			return err
		}
		return nil
	})
}
func (s *Store) SetEnabled(ctx context.Context, id int64, enabled bool) error {
	return s.DB.WithTx(ctx, func(tx *db.Tx) error {
		rows, e := tx.Query("SELECT id FROM users WHERE id=?", strconv.FormatInt(id, 10))
		if e != nil {
			return e
		}
		if len(rows) == 0 {
			return ErrNotFound
		}
		return tx.Exec("UPDATE users SET enabled=? WHERE id=?", boolean(enabled), strconv.FormatInt(id, 10))
	})
}
func (s *Store) DeleteUser(ctx context.Context, id int64) error {
	return s.DB.WithTx(ctx, func(tx *db.Tx) error { return tx.Exec("DELETE FROM users WHERE id=?", strconv.FormatInt(id, 10)) })
}
func (s *Store) RestoreUser(ctx context.Context, u User) error {
	return s.DB.WithTx(ctx, func(tx *db.Tx) error {
		return tx.Exec("INSERT INTO users(id,name,uuid,token,enabled,created_at) VALUES(?,?,?,?,?,?)", strconv.FormatInt(u.ID, 10), u.Name, u.UUID, u.Subscription, boolean(u.Enabled), strconv.FormatInt(u.CreatedAt, 10))
	})
}
func boolean(v bool) string {
	if v {
		return "1"
	}
	return "0"
}
func (s *Store) ReadReality(ctx context.Context) (Reality, error) {
	var r Reality
	err := s.DB.Read(ctx, func(tx *db.Tx) error {
		rows, err := tx.Query("SELECT address,port,target,sni,priv,pub,short_id,enabled FROM config WHERE id=1")
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			return ErrNotFound
		}
		row := rows[0]
		r.Address = row["address"]
		r.Port, _ = strconv.Atoi(row["port"])
		r.Target = row["target"]
		r.SNI = row["sni"]
		r.Private = row["priv"]
		r.Public = row["pub"]
		r.ShortID = row["short_id"]
		r.Enabled = row["enabled"] == "1"
		return nil
	})
	return r, err
}
func (s *Store) SaveReality(ctx context.Context, r Reality) error {
	return s.DB.WithTx(ctx, func(tx *db.Tx) error {
		return tx.Exec("INSERT INTO config(id,address,port,target,sni,priv,pub,short_id,enabled) VALUES(1,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET address=excluded.address,port=excluded.port,target=excluded.target,sni=excluded.sni,priv=excluded.priv,pub=excluded.pub,short_id=excluded.short_id,enabled=excluded.enabled", r.Address, strconv.Itoa(r.Port), r.Target, r.SNI, r.Private, r.Public, r.ShortID, boolean(r.Enabled))
	})
}
func (s *Store) Event(ctx context.Context, action, details string) {
	_ = s.DB.WithTx(ctx, func(tx *db.Tx) error {
		if err := tx.Exec("INSERT INTO events(time,action,details) VALUES(?,?,?)", strconv.FormatInt(time.Now().Unix(), 10), action, details); err != nil {
			return err
		}
		return tx.Exec("DELETE FROM events WHERE id NOT IN (SELECT id FROM events ORDER BY id DESC LIMIT 500)")
	})
}
func (s *Store) Events(ctx context.Context) ([]map[string]string, error) {
	var result []map[string]string
	err := s.DB.Read(ctx, func(tx *db.Tx) error {
		rows, e := tx.Query("SELECT id,time,action,details FROM events ORDER BY id DESC LIMIT 40")
		if e != nil {
			return e
		}
		result = make([]map[string]string, 0, len(rows))
		for _, r := range rows {
			result = append(result, r)
		}
		return nil
	})
	return result, err
}
func (s *Store) VerifyUserToken(ctx context.Context, token string) (User, error) {
	// DB equality is indexed and token entropy prevents enumeration. Keep timing independent
	// when comparing the fetched token to a caller-supplied value.
	u, err := s.ByToken(ctx, token)
	if err != nil {
		return u, err
	}
	if subtle.ConstantTimeCompare([]byte(u.Subscription), []byte(token)) != 1 {
		return User{}, fmt.Errorf("%w: token", ErrNotFound)
	}
	return u, nil
}

// ReplaceState atomically replaces only VPN configuration and VPN users.
// Administrator credentials, sessions and audit history stay local to this installation.
// The caller must serialize updates with other application mutations and validate inputs.
func (s *Store) ReplaceState(ctx context.Context, reality *Reality, users []User) error {
	return s.DB.WithTx(ctx, func(tx *db.Tx) error {
		if err := tx.Exec("DELETE FROM users"); err != nil {
			return err
		}
		if err := tx.Exec("DELETE FROM config"); err != nil {
			return err
		}
		if reality != nil {
			r := reality
			if err := tx.Exec("INSERT INTO config(id,address,port,target,sni,priv,pub,short_id,enabled) VALUES(1,?,?,?,?,?,?,?,?)", r.Address, strconv.Itoa(r.Port), r.Target, r.SNI, r.Private, r.Public, r.ShortID, boolean(r.Enabled)); err != nil {
				return err
			}
		}
		for _, u := range users {
			if err := tx.Exec("INSERT INTO users(id,name,uuid,token,enabled,created_at) VALUES(?,?,?,?,?,?)", strconv.FormatInt(u.ID, 10), u.Name, u.UUID, u.Subscription, boolean(u.Enabled), strconv.FormatInt(u.CreatedAt, 10)); err != nil {
				return err
			}
		}
		return nil
	})
}

// ReplaceFullState commits a complete portable VPN snapshot atomically.
// The administrator, sessions, audit records and machine-specific settings stay intact.
func (s *Store) ReplaceFullState(ctx context.Context, reality *Reality, users []User, profiles []Protocol, creds []ProtocolCredential) error {
	return s.ReplaceFullStateWithManagement(ctx, reality, users, profiles, creds, nil, nil)
}

// ReplaceFullStateWithManagement atomically restores portable VPN data and,
// when supplied, the enrolled node identities and panel log budget. Legacy
// v1/v2 snapshots intentionally leave management settings unchanged.
func (s *Store) ReplaceFullStateWithManagement(ctx context.Context, reality *Reality, users []User, profiles []Protocol, creds []ProtocolCredential, nodes *[]NodeIdentity, budget *int) error {
	return s.ReplaceFullStateWithRoutes(ctx, reality, users, profiles, creds, nodes, budget, nil)
}

// A nil route pointer preserves drafts (legacy backups). A non-nil pointer
// replaces them, including an explicitly empty v4 list. Nodes and references
// are validated and committed together; invalid legacy imports leave no changes.
func (s *Store) ReplaceFullStateWithRoutes(ctx context.Context, reality *Reality, users []User, profiles []Protocol, creds []ProtocolCredential, nodes *[]NodeIdentity, budget *int, policies *[]routes.Policy) error {
	return s.ReplaceFullStateWithProfiles(ctx, reality, users, profiles, creds, nodes, budget, policies, nil)
}

func (s *Store) ReplaceFullStateWithProfiles(ctx context.Context, reality *Reality, users []User, profiles []Protocol, creds []ProtocolCredential, nodes *[]NodeIdentity, budget *int, policies *[]routes.Policy, routingProfiles *[]routingprofile.Profile) error {
	return s.ReplaceFullStateWithUsage(ctx, reality, users, profiles, creds, nodes, budget, policies, routingProfiles, nil)
}

func (s *Store) ReplaceFullStateWithUsage(ctx context.Context, reality *Reality, users []User, profiles []Protocol, creds []ProtocolCredential, nodes *[]NodeIdentity, budget *int, policies *[]routes.Policy, routingProfiles *[]routingprofile.Profile, accounting *usage.State) error {
	if budget != nil && (*budget < 50 || *budget > 1024) {
		return ErrInvalidLogBudget
	}
	return s.DB.WithTx(ctx, func(tx *db.Tx) error {
		previousUsage, usageErr := readUsageState(tx)
		if usageErr != nil {
			return usageErr
		}
		oldRows, usageErr := tx.Query("SELECT id,uuid FROM users")
		if usageErr != nil {
			return usageErr
		}
		oldIdentities := map[int64]string{}
		for _, row := range oldRows {
			id, _ := strconv.ParseInt(row["id"], 10, 64)
			oldIdentities[id] = row["uuid"]
		}

		previous, err := readRoutes(tx)
		if err != nil {
			return err
		}
		for _, q := range []string{"DELETE FROM protocol_credentials", "DELETE FROM protocols", "DELETE FROM users", "DELETE FROM config"} {
			if err := tx.Exec(q); err != nil {
				return err
			}
		}
		if reality != nil {
			r := reality
			if err := tx.Exec("INSERT INTO config(id,address,port,target,sni,priv,pub,short_id,enabled) VALUES(1,?,?,?,?,?,?,?,?)", r.Address, strconv.Itoa(r.Port), r.Target, r.SNI, r.Private, r.Public, r.ShortID, boolean(r.Enabled)); err != nil {
				return err
			}
		}
		for _, u := range users {
			if err := tx.Exec("INSERT INTO users(id,name,uuid,token,enabled,created_at) VALUES(?,?,?,?,?,?)", strconv.FormatInt(u.ID, 10), u.Name, u.UUID, u.Subscription, boolean(u.Enabled), strconv.FormatInt(u.CreatedAt, 10)); err != nil {
				return err
			}
		}
		for _, p := range profiles {
			if err := tx.Exec("INSERT INTO protocols(kind,address,port,sni,target,private_key,public_key,short_id,path,tls_cert,tls_key,obfs_key,enabled) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)", p.Kind, p.Address, strconv.Itoa(p.Port), p.SNI, p.Target, p.PrivateKey, p.PublicKey, p.ShortID, p.Path, p.TLSCert, p.TLSKey, p.ObfsKey, boolean(p.Enabled)); err != nil {
				return err
			}
		}
		for _, c := range creds {
			if err := tx.Exec("INSERT INTO protocol_credentials(user_id,kind,secret,private_key,public_key,address) VALUES(?,?,?,?,?,?)", strconv.FormatInt(c.UserID, 10), c.Kind, c.Secret, c.PrivateKey, c.PublicKey, c.Address); err != nil {
				return err
			}
		}
		if nodes != nil {
			if err := tx.Exec("DELETE FROM telemetry_samples WHERE node_id<>'local'"); err != nil {
				return err
			}
			if err := tx.Exec("DELETE FROM managed_nodes"); err != nil {
				return err
			}
			for _, node := range *nodes {
				if err := tx.Exec("INSERT INTO managed_nodes(id,name,token_hash,created_at,last_seen,xray_state) VALUES(?,?,?,?,0,'unknown')", node.ID, node.Name, node.TokenHash, strconv.FormatInt(node.CreatedAt, 10)); err != nil {
					return err
				}
			}
		}
		if budget != nil {
			if err := tx.Exec("INSERT INTO settings(key,value) VALUES('log_budget_mb',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", strconv.Itoa(*budget)); err != nil {
				return err
			}
		}
		if policies != nil {
			if err := replaceRouteDrafts(tx, *policies, previous); err != nil {
				return err
			}
		} else if err := checkRoutes(tx, previous); err != nil {
			return err
		}
		if routingProfiles != nil {
			if err := replaceRoutingProfiles(tx, *routingProfiles); err != nil {
				return err
			}
		}
		return replaceUsageState(tx, accounting, previousUsage, oldIdentities)
	})
}

// ResetAdminPassword is a local recovery operation. Revoking every active
// administrative session is part of the same transaction as changing the hash.
func (s *Store) ResetAdminPassword(ctx context.Context, hash string) error {
	return s.RecoverAdmin(ctx, hash, true)
}
