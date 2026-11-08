package store

import (
	"context"
	"github.com/arcvpn/arcvpn/internal/db"
	"github.com/arcvpn/arcvpn/internal/usage"
	"strconv"
	"strings"
)

func (s *Store) TrafficUsers(ctx context.Context, ids []int64) ([]usage.TrafficUser, error) {
	if len(ids) < 1 || len(ids) > 200 {
		return nil, usage.ErrInvalid
	}
	args := make([]string, len(ids))
	for i, id := range ids {
		if id < 1 {
			return nil, usage.ErrInvalid
		}
		args[i] = strconv.FormatInt(id, 10)
	}
	users := make([]usage.TrafficUser, 0, len(ids))
	err := s.DB.Read(ctx, func(tx *db.Tx) error {
		rows, err := tx.Query("SELECT id,uuid FROM users WHERE id IN("+strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")+")", args...)
		if err != nil {
			return err
		}
		for _, row := range rows {
			id, _ := strconv.ParseInt(row["id"], 10, 64)
			users = append(users, usage.TrafficUser{ID: id, Identity: row["uuid"]})
		}
		return nil
	})
	return users, err
}
