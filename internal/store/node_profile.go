package store

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/arcvpn/arcvpn/internal/db"
	"strconv"
)

// The per-node Reality private key is encrypted with the same local key used
// for pending configs. Each node receives a unique X25519 keypair.
const nodeProfileSchema = `CREATE TABLE IF NOT EXISTS node_profiles (
 node_id TEXT PRIMARY KEY REFERENCES managed_nodes(id) ON DELETE CASCADE,
 sealed_reality TEXT NOT NULL
)`

func (s *Store) NodeReality(ctx context.Context, id string) (Reality, error) {
	var r Reality
	err := s.DB.Read(ctx, func(tx *db.Tx) error {
		rows, e := tx.Query("SELECT sealed_reality FROM node_profiles WHERE node_id=?", id)
		if e != nil {
			return e
		}
		if len(rows) == 0 {
			return ErrNotFound
		}
		plain, e := s.openNodeConfig(rows[0]["sealed_reality"], "node-reality:"+id)
		if e != nil {
			return e
		}
		var saved struct {
			Reality
			PrivateKey string `json:"private_key"`
		}
		if e := json.Unmarshal(plain, &saved); e != nil {
			return e
		}
		r = saved.Reality
		r.Private = saved.PrivateKey
		if r.Private == "" {
			return errors.New("node private key is missing")
		}
		return nil
	})
	return r, err
}
func (s *Store) PutNodeRealityIfAbsent(ctx context.Context, id string, r Reality) error {
	plain, e := json.Marshal(struct {
		Reality
		PrivateKey string `json:"private_key"`
	}{Reality: r, PrivateKey: r.Private})
	if e != nil {
		return e
	}
	return s.DB.WithTx(ctx, func(tx *db.Tx) error {
		rows, e := tx.Query("SELECT node_id FROM node_control WHERE node_id=?", id)
		if e != nil {
			return e
		}
		if len(rows) == 0 {
			return ErrNotFound
		}
		cipher, e := s.sealNodeConfig(plain, "node-reality:"+id)
		if e != nil {
			return e
		}
		return tx.Exec("INSERT OR IGNORE INTO node_profiles(node_id,sealed_reality) VALUES(?,?)", id, cipher)
	})
}
func (s *Store) UpdateNodeEndpoint(ctx context.Context, id, address string, port int) error {
	if port < 1 || port > 65535 || address == "" {
		return errors.New("invalid endpoint")
	}
	return s.DB.WithTx(ctx, func(tx *db.Tx) error {
		return tx.Exec("UPDATE node_control SET address=?,port=?,published=0,probe='unknown' WHERE node_id=?", address, strconv.Itoa(port), id)
	})
}
