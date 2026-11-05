package store

import (
	"context"
	"github.com/arcvpn/arcvpn/internal/db"
	"github.com/arcvpn/arcvpn/internal/routes"
	"strconv"
)

// NodeIdentity contains the existing agent credential hash (not the raw bearer
// secret). It may be included ONLY in an authenticated encrypted backup.
// Do not expose it from the public node API.
type NodeIdentity struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	TokenHash string `json:"token_hash"`
	CreatedAt int64  `json:"created_at"`
}

func (s *Store) ListNodeIdentities(ctx context.Context) ([]NodeIdentity, error) {
	nodes, _, err := s.RouteBackupState(ctx)
	return nodes, err
}

// Node identities and route references come from the same SQLite snapshot.
func (s *Store) RouteBackupState(ctx context.Context) ([]NodeIdentity, []routes.Policy, error) {
	result := make([]NodeIdentity, 0)
	var policies []routes.Policy
	err := s.DB.Read(ctx, func(tx *db.Tx) error {
		rows, e := tx.Query("SELECT id,name,token_hash,created_at FROM managed_nodes ORDER BY id")
		if e != nil {
			return e
		}
		for _, row := range rows {
			created, _ := strconv.ParseInt(row["created_at"], 10, 64)
			result = append(result, NodeIdentity{ID: row["id"], Name: row["name"], TokenHash: row["token_hash"], CreatedAt: created})
		}
		policies, e = readRoutes(tx)
		if e != nil {
			return e
		}
		return checkRoutes(tx, policies)
	})
	return result, policies, err
}
