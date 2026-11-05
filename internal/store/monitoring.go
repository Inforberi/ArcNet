package store

import (
	"context"
	"errors"
	"strconv"

	"github.com/arcvpn/arcvpn/internal/db"
)

// Monitoring cadence is independent of heartbeat, apply and VPN probes.
const DefaultMonitoringSeconds = 5

func ValidMonitoringSeconds(n int) bool {
	switch n {
	case 5, 10, 15, 30, 60:
		return true
	}
	return false
}

func (s *Store) MonitoringSeconds(ctx context.Context) (int, error) {
	value := DefaultMonitoringSeconds
	err := s.DB.Read(ctx, func(tx *db.Tx) error {
		rows, err := tx.Query("SELECT value FROM settings WHERE key='monitoring_seconds'")
		if err == nil && len(rows) > 0 {
			n, parseErr := strconv.Atoi(rows[0]["value"])
			if parseErr == nil && ValidMonitoringSeconds(n) {
				value = n
			}
		}
		return err
	})
	return value, err
}

func (s *Store) SetMonitoringSeconds(ctx context.Context, n int) error {
	if !ValidMonitoringSeconds(n) {
		return errors.New("unsupported monitoring interval")
	}
	return s.DB.WithTx(ctx, func(tx *db.Tx) error {
		if err := tx.Exec("INSERT INTO settings(key,value) VALUES('monitoring_seconds',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", strconv.Itoa(n)); err != nil {
			return err
		}
		if err := tx.Exec("INSERT INTO events(time,action,details) VALUES(strftime('%s','now'),'monitoring.interval',?)", "monitoring interval seconds: "+strconv.Itoa(n)); err != nil {
			return err
		}
		return tx.Exec("DELETE FROM events WHERE id NOT IN (SELECT id FROM events ORDER BY id DESC LIMIT 500)")
	})
}
