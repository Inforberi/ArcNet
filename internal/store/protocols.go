package store

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strconv"

	"github.com/arcvpn/arcvpn/internal/db"
	"github.com/arcvpn/arcvpn/internal/devices"
)

// Protocol is a logical, local listener. Secret fields are deliberately hidden from API serialization.
// Each protocol has its own listener and cryptographic material.
type Protocol struct {
	ID            string        `json:"id,omitempty"`
	ServerOptions ServerOptions `json:"server_options"`
	ClientOptions ClientOptions `json:"client_options"`
	Fingerprint   string        `json:"fingerprint,omitempty"`
	Kind          string        `json:"kind"`
	Address       string        `json:"address"`
	Port          int           `json:"port"`
	SNI           string        `json:"sni,omitempty"`
	Target        string        `json:"target,omitempty"`
	PrivateKey    string        `json:"-"`
	PublicKey     string        `json:"public_key,omitempty"`
	ShortID       string        `json:"short_id,omitempty"`
	Path          string        `json:"path,omitempty"`
	TLSCert       string        `json:"tls_cert,omitempty"`
	TLSKey        string        `json:"-"`
	ObfsKey       string        `json:"-"`
	Enabled       bool          `json:"enabled"`
}

// ProtocolCredential is per-user access data; the backend never serializes it
// outside an explicitly authenticated export or a protected backup.
type ProtocolCredential struct {
	UserID     int64  `json:"user_id"`
	Kind       string `json:"kind"`
	Secret     string `json:"secret"`
	PrivateKey string `json:"private_key,omitempty"`
	PublicKey  string `json:"public_key,omitempty"`
	Address    string `json:"address,omitempty"`
}

// The historical SQL column kind stores a connection key; rowProtocol separates ID/type.
const profilesSchema = `CREATE TABLE IF NOT EXISTS protocols (
 kind TEXT PRIMARY KEY, address TEXT NOT NULL, port INTEGER NOT NULL, sni TEXT NOT NULL DEFAULT '', target TEXT NOT NULL DEFAULT '',
 private_key TEXT NOT NULL DEFAULT '', public_key TEXT NOT NULL DEFAULT '', short_id TEXT NOT NULL DEFAULT '', path TEXT NOT NULL DEFAULT '',
 tls_cert TEXT NOT NULL DEFAULT '', tls_key TEXT NOT NULL DEFAULT '', obfs_key TEXT NOT NULL DEFAULT '', enabled INTEGER NOT NULL DEFAULT 0
)`
const credentialsAddressIndex = `CREATE UNIQUE INDEX IF NOT EXISTS protocol_credentials_tunnel_address_unique ON protocol_credentials(kind,address) WHERE address != ''`
const credentialsSchema = `CREATE TABLE IF NOT EXISTS protocol_credentials (
 user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE, kind TEXT NOT NULL REFERENCES protocols(kind) ON DELETE CASCADE,
 secret TEXT NOT NULL, private_key TEXT NOT NULL DEFAULT '', public_key TEXT NOT NULL DEFAULT '', address TEXT NOT NULL DEFAULT '',
 PRIMARY KEY(user_id,kind)
)`

func (s *Store) EnsureProtocolSchema(ctx context.Context) error {
	return s.DB.WithTx(ctx, func(tx *db.Tx) error {
		if err := tx.Exec(profilesSchema); err != nil {
			return err
		}
		if err := tx.Exec(credentialsSchema); err != nil {
			return err
		}
		return tx.Exec(credentialsAddressIndex)
	})
}

func rowProtocol(row db.Row) Protocol {
	port, _ := strconv.Atoi(row["port"])
	p := Protocol{Kind: ConnectionKind(row["kind"]), Address: row["address"], Port: port, SNI: row["sni"], Target: row["target"], PrivateKey: row["private_key"], PublicKey: row["public_key"], ShortID: row["short_id"], Path: row["path"], TLSCert: row["tls_cert"], TLSKey: row["tls_key"], ObfsKey: row["obfs_key"], Enabled: row["enabled"] == "1"}
	if IsAdditionalConnection(row["kind"]) {
		p.ID = row["kind"]
	}
	return p
}

const protocolColumns = "kind,address,port,sni,target,private_key,public_key,short_id,path,tls_cert,tls_key,obfs_key,enabled"

func (s *Store) ListProtocols(ctx context.Context) ([]Protocol, error) {
	result := make([]Protocol, 0)
	err := s.DB.Read(ctx, func(tx *db.Tx) error {
		rows, err := tx.Query("SELECT " + protocolColumns + " FROM protocols ORDER BY kind")
		if err != nil {
			return err
		}
		for _, r := range rows {
			p := rowProtocol(r)
			p.Fingerprint, err = readFingerprint(tx, p.ConnectionID())
			if err != nil {
				return err
			}
			p.ServerOptions, err = readServerOptions(tx, p.ConnectionID())
			if err != nil {
				return err
			}
			p.ClientOptions, err = readClientOptions(tx, p.ConnectionID())
			if err != nil {
				return err
			}
			result = append(result, p)
		}
		return nil
	})
	return result, err
}
func (s *Store) GetProtocol(ctx context.Context, kind string) (Protocol, error) {
	var p Protocol
	err := s.DB.Read(ctx, func(tx *db.Tx) error {
		rows, err := tx.Query("SELECT "+protocolColumns+" FROM protocols WHERE kind=?", kind)
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			return ErrNotFound
		}
		p = rowProtocol(rows[0])
		p.Fingerprint, err = readFingerprint(tx, kind)
		if err != nil {
			return err
		}
		p.ServerOptions, err = readServerOptions(tx, kind)
		if err != nil {
			return err
		}
		p.ClientOptions, err = readClientOptions(tx, kind)
		return err
	})
	return p, err
}
func (s *Store) UpsertProtocol(ctx context.Context, p Protocol) error {
	key := p.ConnectionID()
	if p.Kind == "vless-reality" && p.ID == "" {
		return ErrConflict
	}
	if p.ID != "" && (!IsAdditionalConnection(p.ID) || ConnectionKind(p.ID) != p.Kind) {
		return ErrConflict
	}
	return s.DB.WithTx(ctx, func(tx *db.Tx) error {
		if err := saveServerOptions(tx, key, p.ServerOptions); err != nil {
			return err
		}
		if err := saveClientOptions(tx, key, p.ClientOptions); err != nil {
			return err
		}
		if err := saveFingerprint(tx, key, p.Fingerprint); err != nil {
			return err
		}
		return tx.Exec(`INSERT INTO protocols(kind,address,port,sni,target,private_key,public_key,short_id,path,tls_cert,tls_key,obfs_key,enabled)
         VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(kind) DO UPDATE SET address=excluded.address,port=excluded.port,
         sni=excluded.sni,target=excluded.target,private_key=excluded.private_key,public_key=excluded.public_key,
         short_id=excluded.short_id,path=excluded.path,tls_cert=excluded.tls_cert,tls_key=excluded.tls_key,obfs_key=excluded.obfs_key,enabled=excluded.enabled`,
			key, p.Address, strconv.Itoa(p.Port), p.SNI, p.Target, p.PrivateKey, p.PublicKey, p.ShortID, p.Path, p.TLSCert, p.TLSKey, p.ObfsKey, boolean(p.Enabled))
	})
}
func (s *Store) DeleteProtocol(ctx context.Context, kind string) error {
	return s.DB.WithTx(ctx, func(tx *db.Tx) error {
		if IsAdditionalConnection(kind) {
			rows, err := tx.Query("SELECT id,name,preferences FROM users")
			if err != nil {
				return err
			}
			for _, row := range rows {
				var p UserPreferences
				if json.Unmarshal([]byte(row["preferences"]), &p) != nil {
					return ErrInvalidUserPreferences
				}
				changed := false
				if slices.Contains(p.AllowedKinds, kind) {
					p.AllowedKinds = slices.DeleteFunc(p.AllowedKinds, func(value string) bool { return value == kind })
					changed = true
				}
				if p.DirectKind == kind {
					p.DirectKind = ""
					p.AccessMode = "subscription"
					changed = true
				}
				if changed {
					id, e := strconv.ParseInt(row["id"], 10, 64)
					if e != nil {
						return e
					}
					if e = saveUserPreferences(tx, id, row["name"], p, p.Version); e != nil {
						return e
					}
				}
			}
		}
		if err := saveServerOptions(tx, kind, ServerOptions{}); err != nil {
			return err
		}
		if err := saveClientOptions(tx, kind, ClientOptions{}); err != nil {
			return err
		}
		if err := saveFingerprint(tx, kind, ""); err != nil {
			return err
		}
		return tx.Exec("DELETE FROM protocols WHERE kind=?", kind)
	})
}
func rowCredential(row db.Row) ProtocolCredential {
	id, _ := strconv.ParseInt(row["user_id"], 10, 64)
	return ProtocolCredential{UserID: id, Kind: row["kind"], Secret: row["secret"], PrivateKey: row["private_key"], PublicKey: row["public_key"], Address: row["address"]}
}
func (s *Store) ListProtocolCredentials(ctx context.Context) ([]ProtocolCredential, error) {
	result := make([]ProtocolCredential, 0)
	err := s.DB.Read(ctx, func(tx *db.Tx) error {
		rows, err := tx.Query("SELECT user_id,kind,secret,private_key,public_key,address FROM protocol_credentials ORDER BY user_id,kind")
		if err != nil {
			return err
		}
		for _, r := range rows {
			result = append(result, rowCredential(r))
		}
		return nil
	})
	return result, err
}
func (s *Store) CredentialsForUser(ctx context.Context, id int64) ([]ProtocolCredential, error) {
	result := make([]ProtocolCredential, 0)
	err := s.DB.Read(ctx, func(tx *db.Tx) error {
		rows, err := tx.Query("SELECT user_id,kind,secret,private_key,public_key,address FROM protocol_credentials WHERE user_id=? ORDER BY kind", strconv.FormatInt(id, 10))
		if err != nil {
			return err
		}
		for _, r := range rows {
			result = append(result, rowCredential(r))
		}
		return nil
	})
	return result, err
}
func (s *Store) SaveCredentials(ctx context.Context, items []ProtocolCredential) error {
	return s.DB.WithTx(ctx, func(tx *db.Tx) error { return saveCredentials(tx, items) })
}

func saveCredentials(tx *db.Tx, items []ProtocolCredential) error {
	state, err := readDevices(tx)
	if err != nil {
		return err
	}
	used, err := wireGuardClaims(tx, state)
	if err != nil {
		return err
	}
	for _, c := range items {
		if c.Kind == "wireguard" {
			existing, err := tx.Query("SELECT user_id FROM protocol_credentials WHERE kind='wireguard' AND user_id=?", strconv.FormatInt(c.UserID, 10))
			if err != nil {
				return err
			}
			if len(existing) > 0 {
				continue
			}
			if !devices.WireGuardAddress(c.Address) {
				return devices.ErrInvalid
			}
			if used[c.Address] {
				c.Address, err = freeWireGuardAddress(used)
				if err != nil {
					return err
				}
			} else {
				used[c.Address] = true
			}
		}
		if err := tx.Exec(`INSERT INTO protocol_credentials(user_id,kind,secret,private_key,public_key,address) VALUES(?,?,?,?,?,?)
            ON CONFLICT(user_id,kind) DO NOTHING`, strconv.FormatInt(c.UserID, 10), c.Kind, c.Secret, c.PrivateKey, c.PublicKey, c.Address); err != nil {
			return err
		}
	}
	return nil
}
func (s *Store) ReplaceProtocols(ctx context.Context, profiles []Protocol, creds []ProtocolCredential) error {
	return s.DB.WithTx(ctx, func(tx *db.Tx) error {
		if err := clearProtocolServerOptions(tx); err != nil {
			return err
		}
		if err := clearProtocolClientOptions(tx); err != nil {
			return err
		}
		if err := tx.Exec("DELETE FROM settings WHERE key LIKE 'client_fingerprint:vless-xhttp%' OR key LIKE 'client_fingerprint:vless-reality:%'"); err != nil {
			return err
		}
		if err := tx.Exec("DELETE FROM protocol_credentials"); err != nil {
			return err
		}
		if err := tx.Exec("DELETE FROM protocols"); err != nil {
			return err
		}
		for _, p := range profiles {
			if err := saveServerOptions(tx, p.ConnectionID(), p.ServerOptions); err != nil {
				return err
			}
			if err := saveClientOptions(tx, p.ConnectionID(), p.ClientOptions); err != nil {
				return err
			}
			if err := saveFingerprint(tx, p.ConnectionID(), p.Fingerprint); err != nil {
				return err
			}
			if err := tx.Exec("INSERT INTO protocols(kind,address,port,sni,target,private_key,public_key,short_id,path,tls_cert,tls_key,obfs_key,enabled) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)",
				p.ConnectionID(), p.Address, strconv.Itoa(p.Port), p.SNI, p.Target, p.PrivateKey, p.PublicKey, p.ShortID, p.Path, p.TLSCert, p.TLSKey, p.ObfsKey, boolean(p.Enabled)); err != nil {
				return err
			}
		}
		for _, c := range creds {
			if err := tx.Exec("INSERT INTO protocol_credentials(user_id,kind,secret,private_key,public_key,address) VALUES(?,?,?,?,?,?)",
				strconv.FormatInt(c.UserID, 10), c.Kind, c.Secret, c.PrivateKey, c.PublicKey, c.Address); err != nil {
				return err
			}
		}
		return nil
	})
}
func (s *Store) DeleteCredentialsForUser(ctx context.Context, id int64) error {
	return s.DB.WithTx(ctx, func(tx *db.Tx) error {
		return tx.Exec("DELETE FROM protocol_credentials WHERE user_id=?", strconv.FormatInt(id, 10))
	})
}
func (s *Store) CheckProtocolSchema(ctx context.Context) error {
	return s.DB.Read(ctx, func(tx *db.Tx) error {
		for _, table := range []string{"protocols", "protocol_credentials"} {
			rows, err := tx.Query("SELECT name FROM sqlite_master WHERE type='table' AND name=?", table)
			if err != nil {
				return err
			}
			if len(rows) != 1 {
				return errors.New("protocol table missing")
			}
		}
		return nil
	})
}
