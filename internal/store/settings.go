package store

import (
	"context"
	"errors"
	"github.com/arcvpn/arcvpn/internal/db"
	"strconv"
)

const defaultLogBudgetMB = 500

var ErrInvalidLogBudget = errors.New("log budget must be between 50 and 1024 MiB")

func (s *Store) LogBudgetMB(ctx context.Context) (int, error) {
	var budget = defaultLogBudgetMB
	err := s.DB.Read(ctx, func(tx *db.Tx) error {
		rows, err := tx.Query("SELECT value FROM settings WHERE key='log_budget_mb'")
		if err != nil {
			return err
		}
		if len(rows) != 0 {
			n, e := strconv.Atoi(rows[0]["value"])
			if e == nil && n >= 50 && n <= 1024 {
				budget = n
			}
		}
		return nil
	})
	return budget, err
}
func (s *Store) SetLogBudgetMB(ctx context.Context, budget int) error {
	if budget < 50 || budget > 1024 {
		return ErrInvalidLogBudget
	}
	return s.DB.WithTx(ctx, func(tx *db.Tx) error {
		return tx.Exec("INSERT INTO settings(key,value) VALUES('log_budget_mb',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", strconv.Itoa(budget))
	})
}
