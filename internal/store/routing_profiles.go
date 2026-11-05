package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"time"

	"github.com/arcvpn/arcvpn/internal/auth"
	"github.com/arcvpn/arcvpn/internal/db"
	"github.com/arcvpn/arcvpn/internal/routingprofile"
)

const routingProfilesSchema = `CREATE TABLE IF NOT EXISTS routing_profiles (id TEXT PRIMARY KEY, body TEXT NOT NULL, version INTEGER NOT NULL CHECK(version>0),updated_at INTEGER NOT NULL)`

var profileIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,80}$`)

func readRoutingProfiles(tx *db.Tx) ([]routingprofile.Profile, error) {
	rows, err := tx.Query("SELECT id,body,version,updated_at FROM routing_profiles ORDER BY updated_at DESC,id")
	if err != nil {
		return nil, err
	}
	result := []routingprofile.Profile{}
	for _, row := range rows {
		var p routingprofile.Profile
		if err = json.Unmarshal([]byte(row["body"]), &p); err != nil {
			return nil, errors.New("corrupt routing profile")
		}
		version, e := strconv.ParseInt(row["version"], 10, 64)
		if e != nil || version < 1 || p.ID != row["id"] {
			return nil, errors.New("invalid routing profile revision")
		}
		p, err = routingprofile.Normalize(p)
		if err != nil {
			return nil, err
		}
		p.Version = version
		p.UpdatedAt, _ = strconv.ParseInt(row["updated_at"], 10, 64)
		result = append(result, p)
	}
	return result, nil
}
func (s *Store) RoutingProfiles(ctx context.Context) ([]routingprofile.Profile, error) {
	var result []routingprofile.Profile
	err := s.DB.Read(ctx, func(tx *db.Tx) error { var e error; result, e = readRoutingProfiles(tx); return e })
	return result, err
}
func (s *Store) SaveRoutingProfile(ctx context.Context, p routingprofile.Profile, expected int64) (routingprofile.Profile, error) {
	p, err := routingprofile.Normalize(p)
	if err != nil {
		return p, err
	}
	if expected < 0 || expected == math.MaxInt64 {
		return p, fmt.Errorf("%w: версия вне диапазона", routingprofile.ErrInvalid)
	}
	if expected == 0 {
		p.ID, err = auth.UUID()
		if err != nil {
			return p, err
		}
	} else if !profileIDPattern.MatchString(p.ID) {
		return p, routingprofile.ErrInvalid
	}
	err = s.DB.WithTx(ctx, func(tx *db.Tx) error {
		existing, e := readRoutingProfiles(tx)
		if e != nil {
			return e
		}
		found := false
		for _, item := range existing {
			if item.ID == p.ID {
				found = true
				if item.Version != expected {
					return ErrConflict
				}
			}
		}
		if expected > 0 && !found {
			return ErrConflict
		}
		if expected == 0 && len(existing) >= routingprofile.MaxProfiles {
			return fmt.Errorf("%w: не более %d профилей", routingprofile.ErrInvalid, routingprofile.MaxProfiles)
		}
		p.Version = expected + 1
		p.UpdatedAt = time.Now().Unix()
		body, e := json.Marshal(p)
		if e != nil {
			return e
		}
		return tx.Exec("INSERT INTO routing_profiles(id,body,version,updated_at) VALUES(?,?,?,?) ON CONFLICT(id) DO UPDATE SET body=excluded.body,version=excluded.version,updated_at=excluded.updated_at", p.ID, string(body), strconv.FormatInt(p.Version, 10), strconv.FormatInt(p.UpdatedAt, 10))
	})
	return p, err
}
func (s *Store) DeleteRoutingProfile(ctx context.Context, id string, expected int64) error {
	if expected < 1 {
		return routingprofile.ErrInvalid
	}
	return s.DB.WithTx(ctx, func(tx *db.Tx) error {
		rows, e := tx.Query("SELECT version FROM routing_profiles WHERE id=?", id)
		if e != nil {
			return e
		}
		if len(rows) == 0 {
			return ErrNotFound
		}
		if rows[0]["version"] != strconv.FormatInt(expected, 10) {
			return ErrConflict
		}
		return tx.Exec("DELETE FROM routing_profiles WHERE id=?", id)
	})
}
func replaceRoutingProfiles(tx *db.Tx, profiles []routingprofile.Profile) error {
	if len(profiles) > routingprofile.MaxProfiles {
		return routingprofile.ErrInvalid
	}
	old, e := readRoutingProfiles(tx)
	if e != nil {
		return e
	}
	versions := map[string]int64{}
	for _, p := range old {
		versions[p.ID] = p.Version
	}
	if e = tx.Exec("DELETE FROM routing_profiles"); e != nil {
		return e
	}
	seen := map[string]bool{}
	for _, p := range profiles {
		p, e = routingprofile.Normalize(p)
		if e != nil {
			return e
		}
		if !profileIDPattern.MatchString(p.ID) || seen[p.ID] || p.Version < 1 {
			return routingprofile.ErrInvalid
		}
		seen[p.ID] = true
		if versions[p.ID] > p.Version {
			p.Version = versions[p.ID]
		}
		if p.Version >= math.MaxInt64-1 {
			return routingprofile.ErrInvalid
		}
		p.Version++
		p.UpdatedAt = time.Now().Unix()
		body, e := json.Marshal(p)
		if e != nil {
			return e
		}
		if e = tx.Exec("INSERT INTO routing_profiles(id,body,version,updated_at) VALUES(?,?,?,?)", p.ID, string(body), strconv.FormatInt(p.Version, 10), strconv.FormatInt(p.UpdatedAt, 10)); e != nil {
			return e
		}
	}
	return nil
}
