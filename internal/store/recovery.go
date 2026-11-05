package store

import (
	"context"
	"strconv"
	"time"

	"github.com/arcvpn/arcvpn/internal/db"
)

// RecoverAdmin changes only administrator access. VPN users, keys, subscription
// tokens and node identities are not involved in this transaction.
func (s *Store) RecoverAdmin(ctx context.Context, hash string, resetMFA bool) error {
	return s.DB.WithTx(ctx, func(tx *db.Tx) error {
		rows, err := tx.Query("SELECT username FROM admins LIMIT 1")
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			return ErrNotFound
		}
		if err = tx.Exec("UPDATE admins SET hash=?", hash); err != nil {
			return err
		}
		if resetMFA {
			if err = tx.Exec("DELETE FROM admin_mfa"); err != nil {
				return err
			}
		} else {
			if err = tx.Exec("UPDATE admin_mfa SET pending_secret='',pending_until=0"); err != nil {
				return err
			}
		}
		if err = tx.Exec("DELETE FROM sessions"); err != nil {
			return err
		}
		details := "local password recovery; MFA preserved; all sessions revoked"
		if resetMFA {
			details = "local password and MFA recovery; all sessions revoked"
		}
		return recoveryAudit(tx, "admin.recovered", details)
	})
}
func (s *Store) RevokeAdminSessions(ctx context.Context) error {
	return s.DB.WithTx(ctx, func(tx *db.Tx) error {
		rows, err := tx.Query("SELECT username FROM admins LIMIT 1")
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			return ErrNotFound
		}
		if err = tx.Exec("DELETE FROM sessions"); err != nil {
			return err
		}
		return recoveryAudit(tx, "admin.sessions.revoked", "all administrator sessions revoked locally")
	})
}
func recoveryAudit(tx *db.Tx, action, details string) error {
	if err := tx.Exec("INSERT INTO events(time,action,details) VALUES(?,?,?)", strconv.FormatInt(time.Now().Unix(), 10), action, details); err != nil {
		return err
	}
	return tx.Exec("DELETE FROM events WHERE id NOT IN (SELECT id FROM events ORDER BY id DESC LIMIT 500)")
}

type RecoverySettings struct {
	Username       string       `json:"username"`
	MFAEnabled     bool         `json:"mfa_enabled"`
	ActiveSessions int          `json:"active_sessions"`
	PanelLogMB     int          `json:"panel_log_mb"`
	VPN            *RecoveryVPN `json:"vpn,omitempty"`
}
type RecoveryVPN struct {
	Address string `json:"address"`
	Port    int    `json:"port"`
	Enabled bool   `json:"enabled"`
}

// The projection deliberately never selects hashes, keys, user credentials,
// tokens, MFA seeds or session values.
func (s *Store) RecoverySettings(ctx context.Context) (RecoverySettings, error) {
	result := RecoverySettings{PanelLogMB: defaultLogBudgetMB}
	err := s.DB.Read(ctx, func(tx *db.Tx) error {
		rows, err := tx.Query("SELECT username FROM admins LIMIT 1")
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			return ErrNotFound
		}
		result.Username = rows[0]["username"]
		rows, err = tx.Query("SELECT id FROM admin_mfa WHERE secret<>''")
		if err != nil {
			return err
		}
		result.MFAEnabled = len(rows) > 0
		rows, err = tx.Query("SELECT COUNT(*) AS count FROM sessions WHERE expires>?", strconv.FormatInt(time.Now().Unix(), 10))
		if err != nil {
			return err
		}
		result.ActiveSessions, _ = strconv.Atoi(rows[0]["count"])
		rows, err = tx.Query("SELECT value FROM settings WHERE key='log_budget_mb'")
		if err != nil {
			return err
		}
		if len(rows) > 0 {
			value, err := strconv.Atoi(rows[0]["value"])
			if err != nil || value < 50 || value > 1024 {
				return ErrInvalidLogBudget
			}
			result.PanelLogMB = value
		}
		rows, err = tx.Query("SELECT address,port,enabled FROM config WHERE id=1")
		if err != nil {
			return err
		}
		if len(rows) > 0 {
			port, err := strconv.Atoi(rows[0]["port"])
			if err != nil {
				return err
			}
			result.VPN = &RecoveryVPN{Address: rows[0]["address"], Port: port, Enabled: rows[0]["enabled"] == "1"}
		}
		return nil
	})
	return result, err
}
