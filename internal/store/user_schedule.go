package store

import (
	"context"
	"github.com/arcvpn/arcvpn/internal/db"
	"github.com/arcvpn/arcvpn/internal/usage"
	"strconv"
	"time"
)

func setPackageSchedule(tx *db.Tx, id int64, schedule usage.ResetSchedule, now time.Time) error {
	packages, err := readPackages(tx)
	if err != nil {
		return err
	}
	p, ok := packages[id]
	if !ok {
		return ErrNotFound
	}
	if schedule.Frequency == "none" {
		p.ResetSchedule = nil
		return savePackage(tx, p)
	}
	schedule.NextAt, err = schedule.Next(now)
	if err != nil {
		return err
	}
	// Scheduled reset is local-only, including packages without quota or expiry.
	for _, query := range []string{"SELECT id FROM managed_nodes LIMIT 1", "SELECT hash FROM node_enrollments LIMIT 1", "SELECT kind FROM protocols WHERE enabled=1 AND kind NOT IN('vless-xhttp','hysteria2','wireguard') LIMIT 1"} {
		rows, err := tx.Query(query)
		if err != nil {
			return err
		}
		if len(rows) > 0 {
			return ErrUsageUnsupported
		}
	}
	p.ResetSchedule = &schedule
	return savePackage(tx, p)
}

// An overdue schedule advances once, to the next future boundary, after a fresh collection.
func (s *Store) ResetDuePackages(ctx context.Context, now time.Time) (int, error) {
	count := 0
	err := s.DB.WithTx(ctx, func(tx *db.Tx) error {
		packages, err := readPackages(tx)
		if err != nil {
			return err
		}
		due := false
		for _, p := range packages {
			if p.ResetSchedule != nil && p.ResetSchedule.NextAt <= now.Unix() {
				due = true
				break
			}
		}
		if !due {
			return nil
		}
		if err = requireNoCascadeLeases(tx); err != nil {
			return err
		}
		rows, err := tx.Query("SELECT (" + accountingLastSQL + ") AS last")
		if err != nil {
			return err
		}
		last, _ := strconv.ParseInt(rows[0]["last"], 10, 64)
		if last <= 0 || now.Unix()-last < 0 || now.Unix()-last > usage.FreshFor {
			return nil
		}
		nodes, err := tx.Query("SELECT id FROM managed_nodes LIMIT 1")
		if err != nil {
			return err
		}
		if len(nodes) > 0 {
			return ErrUsageUnsupported
		}
		for _, p := range packages {
			if p.ResetSchedule == nil || p.ResetSchedule.NextAt > now.Unix() {
				continue
			}
			next, err := p.ResetSchedule.Next(now)
			if err != nil {
				return err
			}
			p.Upload = 0
			p.Download = 0
			p.Period++
			p.PeriodStartedAt = now.Unix()
			p.Version++
			p.Incomplete = false
			p.NeedsReview = false
			p.ResetSchedule.NextAt = next
			if err = savePackage(tx, p); err != nil {
				return err
			}
			count++
		}
		if count > 0 {
			return pendingUserRuntime(tx)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return count, nil
}
