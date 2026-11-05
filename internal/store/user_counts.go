package store

import (
	"context"
	"strconv"

	"github.com/arcvpn/arcvpn/internal/db"
)

// UserCounts avoids allocating every UUID and subscription token for a dashboard
// that only needs two numbers. No new index is required for this bounded query.
func (s *Store) UserCounts(ctx context.Context) (int, int, error) {
	var total, enabled int
	err := s.DB.Read(ctx, func(tx *db.Tx) error {
		rows, err := tx.Query("SELECT COUNT(*) AS total,COALESCE(SUM(CASE WHEN enabled=1 THEN 1 ELSE 0 END),0) AS enabled FROM users")
		if err != nil {
			return err
		}
		total, err = strconv.Atoi(rows[0]["total"])
		if err != nil {
			return err
		}
		enabled, err = strconv.Atoi(rows[0]["enabled"])
		return err
	})
	return total, enabled, err
}
