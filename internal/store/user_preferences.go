package store

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/arcvpn/arcvpn/internal/db"
)

var ErrInvalidUserPreferences = errors.New("invalid user preferences")

// Nil AllowedKinds preserves legacy access; an explicit empty list denies every local protocol.
type UserPreferences struct {
	Version      int64    `json:"version"`
	Group        string   `json:"group"`
	Comment      string   `json:"comment"`
	Title        string   `json:"title"`
	AccessMode   string   `json:"access_mode"`
	DirectKind   string   `json:"direct_kind"`
	AllowedKinds []string `json:"allowed_kinds"`
}

func (u User) Allows(kind string) bool {
	return u.Preferences.AllowedKinds == nil || slices.Contains(u.Preferences.AllowedKinds, kind)
}
func (p UserPreferences) Valid() bool {
	for _, v := range []struct {
		s string
		n int
	}{{p.Group, 50}, {p.Comment, 220}, {p.Title, 120}} {
		if !utf8.ValidString(v.s) || utf8.RuneCountInString(v.s) > v.n || strings.IndexFunc(v.s, unicode.IsControl) >= 0 {
			return false
		}
	}
	if p.Version < 0 || p.Version >= 1<<53 {
		return false
	}
	if p.AccessMode != "" && p.AccessMode != "subscription" && p.AccessMode != "direct" {
		return false
	}
	valid := []string{"vless-reality", "vless-xhttp", "hysteria2", "wireguard"}
	if len(p.AllowedKinds) > len(valid) {
		return false
	}
	seen := map[string]bool{}
	for _, kind := range p.AllowedKinds {
		if !slices.Contains(valid, kind) || seen[kind] {
			return false
		}
		seen[kind] = true
	}
	if p.DirectKind != "" && (!slices.Contains(valid, p.DirectKind) || (p.AllowedKinds != nil && !slices.Contains(p.AllowedKinds, p.DirectKind))) {
		return false
	}
	return p.AccessMode != "direct" || p.DirectKind != ""
}
func preferencesJSON(p UserPreferences) string { b, _ := json.Marshal(p); return string(b) }
func migrateUserPreferences(tx *db.Tx) error {
	cols, err := tx.Query("PRAGMA table_info(users)")
	if err != nil {
		return err
	}
	for _, c := range cols {
		if c["name"] == "preferences" {
			return nil
		}
	}
	return tx.Exec("ALTER TABLE users ADD COLUMN preferences TEXT NOT NULL DEFAULT '{}'")
}
func (s *Store) SaveUserPreferences(ctx context.Context, id int64, name string, p UserPreferences, expected int64) error {
	return s.DB.WithTx(ctx, func(tx *db.Tx) error { return saveUserPreferences(tx, id, name, p, expected) })
}

func saveUserPreferences(tx *db.Tx, id int64, name string, p UserPreferences, expected int64) error {
	if !p.Valid() || !validUserName(name) {
		return ErrInvalidUserPreferences
	}

	if err := requireNoCascadeLeases(tx); err != nil {
		return err
	}
	rows, err := tx.Query("SELECT preferences FROM users WHERE id=?", strconv.FormatInt(id, 10))
	if err != nil {
		return err
	}
	if len(rows) != 1 {
		return ErrNotFound
	}
	var old UserPreferences
	if json.Unmarshal([]byte(rows[0]["preferences"]), &old) != nil {
		return ErrInvalidUserPreferences
	}
	if old.Version != expected {
		return ErrConflict
	}
	if p.AllowedKinds != nil {
		for _, table := range []string{"managed_nodes", "node_enrollments"} {
			rows, e := tx.Query("SELECT COUNT(*) AS n FROM " + table)
			if e != nil {
				return e
			}
			if rows[0]["n"] != "0" {
				return ErrUserAssignmentsUnavailable
			}
		}
	}
	p.Version = expected + 1
	if !p.Valid() {
		return ErrInvalidUserPreferences
	}
	if err := tx.Exec("UPDATE users SET name=?,preferences=? WHERE id=?", name, preferencesJSON(p), strconv.FormatInt(id, 10)); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return ErrConflict
		}
		return err
	}
	return nil
}

var ErrUserAssignmentsUnavailable = errors.New("user assignments require local-only installation without nodes or pending enrollment")

func requireUnrestrictedUsers(tx *db.Tx) error {
	rows, err := tx.Query("SELECT preferences FROM users")
	if err != nil {
		return err
	}
	for _, row := range rows {
		var p UserPreferences
		if json.Unmarshal([]byte(row["preferences"]), &p) != nil || p.AllowedKinds != nil {
			return ErrUserAssignmentsUnavailable
		}
	}
	packages, err := readPackages(tx)
	if err != nil {
		return err
	}
	for _, p := range packages {
		if p.ResetSchedule != nil {
			return ErrUsageUnsupported
		}
	}
	return nil
}
