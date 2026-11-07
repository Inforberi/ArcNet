package store

import "github.com/arcvpn/arcvpn/internal/db"

// Client-only export preferences use the existing settings table, not runtime secrets.
func readFingerprint(tx *db.Tx, kind string) (string, error) {
	rows, err := tx.Query("SELECT value FROM settings WHERE key=?", "client_fingerprint:"+kind)
	if err != nil || len(rows) == 0 {
		return "", err
	}
	return rows[0]["value"], nil
}

func saveFingerprint(tx *db.Tx, kind, value string) error {
	if value == "" {
		return tx.Exec("DELETE FROM settings WHERE key=?", "client_fingerprint:"+kind)
	}
	return tx.Exec("INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", "client_fingerprint:"+kind, value)
}
