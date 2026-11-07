package store

import (
	"context"
	"crypto/subtle"
	"strconv"
	"time"

	"github.com/arcvpn/arcvpn/internal/db"
)

// Only hashes are stored; the setup code is never part of portable VPN backups.
func (s *Store) BeginAdminSetup(ctx context.Context, hash string, expires time.Time) error {
	return s.DB.WithTx(ctx, func(tx *db.Tx) error {
		admins, err := tx.Query("SELECT username FROM admins LIMIT 1")
		if err != nil {
			return err
		}
		if len(admins) != 0 {
			return ErrConflict
		}
		if err = tx.Exec("INSERT INTO settings(key,value) VALUES('admin_setup_hash',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", hash); err != nil {
			return err
		}
		return tx.Exec("INSERT INTO settings(key,value) VALUES('admin_setup_until',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", strconv.FormatInt(expires.Unix(), 10))
	})
}

func (s *Store) AdminSetupAvailable(ctx context.Context) (bool, error) {
	var available bool
	err := s.DB.Read(ctx, func(tx *db.Tx) error {
		rows, err := tx.Query("SELECT value FROM settings WHERE key='admin_setup_until' AND NOT EXISTS(SELECT 1 FROM admins)")
		if err != nil {
			return err
		}
		if len(rows) == 1 {
			until, _ := strconv.ParseInt(rows[0]["value"], 10, 64)
			available = until > time.Now().Unix()
		}
		return nil
	})
	return available, err
}

func (s *Store) CompleteAdminSetup(ctx context.Context, codeHash, username, passwordHash string) error {
	return s.DB.WithTx(ctx, func(tx *db.Tx) error {
		admins, err := tx.Query("SELECT username FROM admins LIMIT 1")
		if err != nil {
			return err
		}
		if len(admins) != 0 {
			return ErrConflict
		}
		rows, err := tx.Query("SELECT key,value FROM settings WHERE key IN ('admin_setup_hash','admin_setup_until')")
		if err != nil {
			return err
		}
		values := map[string]string{}
		for _, row := range rows {
			values[row["key"]] = row["value"]
		}
		until, _ := strconv.ParseInt(values["admin_setup_until"], 10, 64)
		if until <= time.Now().Unix() || len(codeHash) != 64 || subtle.ConstantTimeCompare([]byte(codeHash), []byte(values["admin_setup_hash"])) != 1 {
			return ErrConflict
		}
		if err = tx.Exec("INSERT INTO admins(username,hash) VALUES(?,?)", username, passwordHash); err != nil {
			return err
		}
		return tx.Exec("DELETE FROM settings WHERE key IN ('admin_setup_hash','admin_setup_until')")
	})
}

func (s *Store) ChangeAdminCredentials(ctx context.Context, previousHash, username, nextHash string) error {
	return s.DB.WithTx(ctx, func(tx *db.Tx) error {
		rows, err := tx.Query("SELECT hash FROM admins LIMIT 1")
		if err != nil {
			return err
		}
		if len(rows) != 1 || subtle.ConstantTimeCompare([]byte(previousHash), []byte(rows[0]["hash"])) != 1 {
			return ErrConflict
		}
		if err = tx.Exec("UPDATE admins SET username=?,hash=?", username, nextHash); err != nil {
			return err
		}
		if err = tx.Exec("UPDATE admin_mfa SET pending_secret='',pending_until=0"); err != nil {
			return err
		}
		if err = tx.Exec("DELETE FROM sessions"); err != nil {
			return err
		}
		return recoveryAudit(tx, "admin.credentials.changed", "Administrator changed credentials; MFA preserved; sessions revoked")
	})
}
