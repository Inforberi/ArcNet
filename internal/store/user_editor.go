package store

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/arcvpn/arcvpn/internal/db"
	"github.com/arcvpn/arcvpn/internal/usage"
)

type UserPackageEdit struct {
	Schedule        *usage.ResetSchedule `json:"schedule"`
	QuotaBytes      int64                `json:"quota_bytes"`
	ExpiresAt       int64                `json:"expires_at"`
	ExpectedVersion int64                `json:"expected_version"`
	ResetUsage      bool                 `json:"reset_usage"`
}
type UserEdit struct {
	Name            string           `json:"name"`
	Preferences     UserPreferences  `json:"preferences"`
	ExpectedVersion int64            `json:"expected_version"`
	ExpectedEnabled bool             `json:"expected_enabled"`
	Enabled         bool             `json:"enabled"`
	Package         *UserPackageEdit `json:"package"`
}
type UserSelection struct {
	ID              int64 `json:"id"`
	ExpectedVersion int64 `json:"expected_version"`
	ExpectedEnabled bool  `json:"expected_enabled"`
}

func (s *Store) UserEditor(ctx context.Context, id int64) (User, usage.Package, error) {
	var u User
	var p usage.Package
	err := s.DB.Read(ctx, func(tx *db.Tx) error {
		rows, err := tx.Query(userQuery+" WHERE id=?", strconv.FormatInt(id, 10))
		if err != nil {
			return err
		}
		if len(rows) != 1 {
			return ErrNotFound
		}
		u = userFromRow(rows[0])
		packages, err := readPackages(tx)
		if err != nil {
			return err
		}
		p = packages[id]
		return nil
	})
	return u, p, err
}
func checkUserSelection(tx *db.Tx, item UserSelection) error {
	rows, err := tx.Query("SELECT enabled,preferences FROM users WHERE id=?", strconv.FormatInt(item.ID, 10))
	if err != nil {
		return err
	}
	if len(rows) != 1 {
		return ErrNotFound
	}
	var p UserPreferences
	if json.Unmarshal([]byte(rows[0]["preferences"]), &p) != nil {
		return ErrInvalidUserPreferences
	}
	if p.Version != item.ExpectedVersion || (rows[0]["enabled"] == "1") != item.ExpectedEnabled {
		return ErrConflict
	}
	return nil
}
func pendingUserRuntime(tx *db.Tx) error {
	return tx.Exec("INSERT INTO settings(key,value) VALUES('usage_runtime','apply_failed') ON CONFLICT(key) DO UPDATE SET value=excluded.value")
}
func (s *Store) SaveUserEditor(ctx context.Context, id int64, in UserEdit, runtimeChanged bool) error {
	return s.DB.WithTx(ctx, func(tx *db.Tx) error {
		if err := checkUserSelection(tx, UserSelection{id, in.ExpectedVersion, in.ExpectedEnabled}); err != nil {
			return err
		}
		if err := saveUserPreferences(tx, id, in.Name, in.Preferences, in.ExpectedVersion); err != nil {
			return err
		}
		if in.Package != nil {
			p := in.Package
			if err := saveUserPackage(tx, id, p.QuotaBytes, p.ExpiresAt, p.ExpectedVersion, p.ResetUsage); err != nil {
				return err
			}
			if p.Schedule != nil {
				if err := setPackageSchedule(tx, id, *p.Schedule, time.Now()); err != nil {
					return err
				}
			}
		}
		if err := tx.Exec("UPDATE users SET enabled=? WHERE id=?", boolean(in.Enabled), strconv.FormatInt(id, 10)); err != nil {
			return err
		}
		if runtimeChanged {
			return pendingUserRuntime(tx)
		}
		return nil
	})
}
func (s *Store) BulkUserAccess(ctx context.Context, items []UserSelection, enabled bool) error {
	if len(items) == 0 || len(items) > 100 {
		return ErrInvalidUserPreferences
	}
	return s.DB.WithTx(ctx, func(tx *db.Tx) error {
		if err := requireNoCascadeLeases(tx); err != nil {
			return err
		}
		seen := map[int64]bool{}
		for _, item := range items {
			if item.ID < 1 || seen[item.ID] {
				return ErrInvalidUserPreferences
			}
			seen[item.ID] = true
			if err := checkUserSelection(tx, item); err != nil {
				return err
			}
			rows, err := tx.Query("SELECT preferences FROM users WHERE id=?", strconv.FormatInt(item.ID, 10))
			if err != nil {
				return err
			}
			var p UserPreferences
			if err = json.Unmarshal([]byte(rows[0]["preferences"]), &p); err != nil {
				return err
			}
			p.Version++
			if !p.Valid() {
				return ErrInvalidUserPreferences
			}
			if err = tx.Exec("UPDATE users SET enabled=?,preferences=? WHERE id=?", boolean(enabled), preferencesJSON(p), strconv.FormatInt(item.ID, 10)); err != nil {
				return err
			}
		}
		return pendingUserRuntime(tx)
	})
}
