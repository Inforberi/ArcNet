package store

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/arcvpn/arcvpn/internal/db"
	"github.com/arcvpn/arcvpn/internal/mfa"
)

var ErrInvalidOTP = errors.New("invalid or previously used one-time password")
var ErrMFADisabled = errors.New("two-factor authentication is disabled")
var ErrMFAEnabled = errors.New("two-factor authentication is already enabled")

// MFAStatus never exposes stored secrets to an API client.
func (s *Store) MFAStatus(ctx context.Context) (bool, error) {
	var active bool
	err := s.DB.Read(ctx, func(tx *db.Tx) error {
		rows, e := tx.Query("SELECT secret FROM admin_mfa WHERE id=1")
		if e != nil {
			return e
		}
		active = len(rows) == 1 && rows[0]["secret"] != ""
		return nil
	})
	return active, err
}
func (s *Store) BeginMFA(ctx context.Context, secret string, until time.Time) error {
	return s.DB.WithTx(ctx, func(tx *db.Tx) error {
		rows, e := tx.Query("SELECT secret FROM admin_mfa WHERE id=1")
		if e != nil {
			return e
		}
		if len(rows) != 0 && rows[0]["secret"] != "" {
			return ErrMFAEnabled
		}
		return tx.Exec("INSERT INTO admin_mfa(id,secret,pending_secret,pending_until,last_step) VALUES(1,'',?,?,-1) ON CONFLICT(id) DO UPDATE SET pending_secret=excluded.pending_secret,pending_until=excluded.pending_until", secret, strconv.FormatInt(until.Unix(), 10))
	})
}
func (s *Store) ConfirmMFA(ctx context.Context, code string, now time.Time) error {
	return s.DB.WithTx(ctx, func(tx *db.Tx) error {
		rows, e := tx.Query("SELECT secret,pending_secret,pending_until FROM admin_mfa WHERE id=1")
		if e != nil {
			return e
		}
		if len(rows) != 1 || rows[0]["secret"] != "" {
			return ErrInvalidOTP
		}
		expiry, e := strconv.ParseInt(rows[0]["pending_until"], 10, 64)
		if e != nil || expiry <= now.Unix() {
			return ErrInvalidOTP
		}
		step := mfa.Verify(rows[0]["pending_secret"], code, now, -1)
		if step < 0 {
			return ErrInvalidOTP
		}
		if e = tx.Exec("UPDATE admin_mfa SET secret=pending_secret,pending_secret='',pending_until=0,last_step=? WHERE id=1", strconv.FormatInt(step, 10)); e != nil {
			return e
		}
		return tx.Exec("DELETE FROM sessions")
	})
}

// CheckLoginMFA consumes a valid TOTP in the same SQLite write transaction.
// Password verification must already have succeeded before this call.
func (s *Store) CheckLoginMFA(ctx context.Context, code string, now time.Time) (bool, error) {
	used := false
	err := s.DB.WithTx(ctx, func(tx *db.Tx) error {
		rows, e := tx.Query("SELECT secret,last_step FROM admin_mfa WHERE id=1")
		if e != nil {
			return e
		}
		if len(rows) == 0 || rows[0]["secret"] == "" {
			used = true
			return nil
		}
		last, e := strconv.ParseInt(rows[0]["last_step"], 10, 64)
		if e != nil {
			return e
		}
		step := mfa.Verify(rows[0]["secret"], code, now, last)
		if step < 0 {
			return nil
		}
		if e = tx.Exec("UPDATE admin_mfa SET last_step=? WHERE id=1", strconv.FormatInt(step, 10)); e != nil {
			return e
		}
		used = true
		return nil
	})
	return used, err
}
func (s *Store) DisableMFA(ctx context.Context, code string, now time.Time) error {
	return s.DB.WithTx(ctx, func(tx *db.Tx) error {
		rows, e := tx.Query("SELECT secret,last_step FROM admin_mfa WHERE id=1")
		if e != nil {
			return e
		}
		if len(rows) == 0 || rows[0]["secret"] == "" {
			return ErrMFADisabled
		}
		last, e := strconv.ParseInt(rows[0]["last_step"], 10, 64)
		if e != nil {
			return e
		}
		if mfa.Verify(rows[0]["secret"], code, now, last) < 0 {
			return ErrInvalidOTP
		}
		if e = tx.Exec("DELETE FROM admin_mfa"); e != nil {
			return e
		}
		return tx.Exec("DELETE FROM sessions")
	})
}
