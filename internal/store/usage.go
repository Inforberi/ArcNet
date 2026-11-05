package store

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/arcvpn/arcvpn/internal/db"
	"github.com/arcvpn/arcvpn/internal/usage"
)

const packagesSchema = `CREATE TABLE IF NOT EXISTS user_packages(user_id INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,body TEXT NOT NULL)`
const checkpointsSchema = `CREATE TABLE IF NOT EXISTS traffic_checkpoints(user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,stream TEXT NOT NULL,body TEXT NOT NULL,PRIMARY KEY(user_id,stream))`
const debitsSchema = `CREATE TABLE IF NOT EXISTS traffic_debits(user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,period INTEGER NOT NULL,bucket INTEGER NOT NULL,upload INTEGER NOT NULL,download INTEGER NOT NULL,PRIMARY KEY(user_id,period,bucket))`

func readPackages(tx *db.Tx) (map[int64]usage.Package, error) {
	rows, err := tx.Query("SELECT id,created_at,body FROM users LEFT JOIN user_packages ON users.id=user_packages.user_id ORDER BY id")
	if err != nil {
		return nil, err
	}
	items := make(map[int64]usage.Package, len(rows))
	for _, row := range rows {
		p, err := packageRow(row)
		if err != nil {
			return nil, err
		}
		items[p.UserID] = p
	}
	return items, nil
}
func savePackage(tx *db.Tx, p usage.Package) error {
	if err := p.Validate(); err != nil {
		return err
	}
	blob, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return tx.Exec("INSERT INTO user_packages(user_id,body) VALUES(?,?) ON CONFLICT(user_id) DO UPDATE SET body=excluded.body", strconv.FormatInt(p.UserID, 10), string(blob))
}

func packageRow(row db.Row) (usage.Package, error) {
	id, _ := strconv.ParseInt(row["id"], 10, 64)
	created, _ := strconv.ParseInt(row["created_at"], 10, 64)
	p := usage.Package{UserID: id, Period: 1, PeriodStartedAt: created}
	if row["body"] != "" {
		if err := json.Unmarshal([]byte(row["body"]), &p); err != nil {
			return p, err
		}
	}
	if p.UserID != id || p.Validate() != nil {
		return p, usage.ErrInvalid
	}
	return p, nil
}
func (s *Store) Package(ctx context.Context, id int64) (usage.Package, error) {
	var p usage.Package
	err := s.DB.Read(ctx, func(tx *db.Tx) error {
		rows, err := tx.Query("SELECT users.id,users.created_at,user_packages.body FROM users LEFT JOIN user_packages ON users.id=user_packages.user_id WHERE users.id=?", strconv.FormatInt(id, 10))
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			return ErrNotFound
		}
		p, err = packageRow(rows[0])
		return err
	})
	return p, err
}
func (s *Store) Packages(ctx context.Context) (map[int64]usage.Package, error) {
	var result map[int64]usage.Package
	err := s.DB.Read(ctx, func(tx *db.Tx) error { var err error; result, err = readPackages(tx); return err })
	return result, err
}
func (s *Store) AccountingFresh(ctx context.Context) (bool, int64, error) {
	var last int64
	err := s.DB.Read(ctx, func(tx *db.Tx) error {
		rows, err := tx.Query("SELECT value FROM settings WHERE key='xray_accounting_last_success'")
		if err == nil && len(rows) > 0 {
			last, _ = strconv.ParseInt(rows[0]["value"], 10, 64)
		}
		return err
	})
	age := time.Now().Unix() - last
	return last > 0 && age >= 0 && age <= usage.FreshFor, last, err
}
func (s *Store) SetAccountingState(ctx context.Context, state string) error {
	if state != "applied" && state != "apply_failed" && state != "unverified" && state != "source_unavailable" {
		return usage.ErrInvalid
	}
	return s.DB.WithTx(ctx, func(tx *db.Tx) error {
		return tx.Exec("INSERT INTO settings(key,value) VALUES('usage_runtime',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", state)
	})
}
func (s *Store) AccountingState(ctx context.Context) (string, error) {
	state := "unverified"
	err := s.DB.Read(ctx, func(tx *db.Tx) error {
		rows, err := tx.Query("SELECT value FROM settings WHERE key='usage_runtime'")
		if err == nil && len(rows) > 0 {
			state = rows[0]["value"]
		}
		return err
	})
	return state, err
}

// RecordUsage atomically persists checkpoints, period totals and minute debit
// history. It is internal-only: no browser or remote bearer can submit bytes.
func (s *Store) RecordUsage(ctx context.Context, counters []usage.Counter, observedAt int64) error {
	return s.recordUsage(ctx, counters, observedAt, "")
}

// RecordUsageAfterApply accepts a new epoch only after an observed, controlled apply.
func (s *Store) RecordUsageAfterApply(ctx context.Context, counters []usage.Counter, observedAt int64, previousEpoch string) error {
	return s.recordUsage(ctx, counters, observedAt, previousEpoch)
}

func (s *Store) recordUsage(ctx context.Context, counters []usage.Counter, observedAt int64, previousEpoch string) error {
	if len(counters) > 20000 || observedAt < 1 || observedAt > time.Now().Unix()+5 || observedAt < time.Now().Unix()-20 {
		return usage.ErrInvalid
	}
	return s.DB.WithTx(ctx, func(tx *db.Tx) error {
		packages, err := readPackages(tx)
		if err != nil {
			return err
		}
		users, err := tx.Query("SELECT id,uuid FROM users")
		if err != nil {
			return err
		}
		identities := map[int64]string{}
		for _, u := range users {
			id, _ := strconv.ParseInt(u["id"], 10, 64)
			identities[id] = u["uuid"]
		}
		rows, err := tx.Query("SELECT user_id,stream,body FROM traffic_checkpoints")
		if err != nil {
			return err
		}
		previous := map[string]usage.Counter{}
		key := func(id int64, stream string) string { return strconv.FormatInt(id, 10) + "/" + stream }
		for _, row := range rows {
			var c usage.Counter
			if json.Unmarshal([]byte(row["body"]), &c) != nil || c.Validate() != nil {
				return usage.ErrInvalid
			}
			previous[key(c.UserID, c.Stream)] = c
		}
		seen := map[string]bool{}
		for _, c := range counters {
			k := key(c.UserID, c.Stream)
			if seen[k] || c.Validate() != nil || c.ObservedAt != observedAt || identities[c.UserID] != c.Identity {
				return usage.ErrInvalid
			}
			seen[k] = true
			var prev *usage.Counter
			if value, ok := previous[k]; ok {
				prev = &value
			}
			up, down, err := usage.Delta(prev, c)
			if err != nil {
				return err
			}
			if prev != nil && (c.ObservedAt < prev.ObservedAt || (c.ObservedAt == prev.ObservedAt && c.Epoch == prev.Epoch)) {
				continue
			}
			// A successful batch refreshes source liveness below. Idle users do
			// not need identical JSON/checkpoint writes every five seconds.
			if prev != nil && prev.Epoch == c.Epoch && up == 0 && down == 0 && c.ObservedAt-prev.ObservedAt < 60 {
				continue
			}
			p, ok := packages[c.UserID]
			if !ok {
				return ErrNotFound
			}
			if up > usage.MaxBytes-p.Upload-p.Download || down > usage.MaxBytes-p.Upload-p.Download-up {
				return usage.ErrInvalid
			}
			p.Upload += up
			p.Download += down
			if prev != nil && prev.Epoch != c.Epoch {
				p.Incomplete = true
				if previousEpoch == "" || prev.Epoch != previousEpoch {
					p.NeedsReview = true
				}
			}
			p.ObservedAt = observedAt
			packages[c.UserID] = p
			if err := savePackage(tx, p); err != nil {
				return err
			}
			body, _ := json.Marshal(c)
			if err := tx.Exec("INSERT INTO traffic_checkpoints(user_id,stream,body) VALUES(?,?,?) ON CONFLICT(user_id,stream) DO UPDATE SET body=excluded.body", strconv.FormatInt(c.UserID, 10), c.Stream, string(body)); err != nil {
				return err
			}
			if up+down > 0 {
				if err := tx.Exec("INSERT INTO traffic_debits(user_id,period,bucket,upload,download) VALUES(?,?,?,?,?) ON CONFLICT(user_id,period,bucket) DO UPDATE SET upload=upload+excluded.upload,download=download+excluded.download", strconv.FormatInt(c.UserID, 10), strconv.FormatInt(p.Period, 10), strconv.FormatInt(observedAt/60, 10), strconv.FormatInt(up, 10), strconv.FormatInt(down, 10)); err != nil {
					return err
				}
			}
		}
		if err := tx.Exec("INSERT INTO settings(key,value) VALUES('xray_accounting_last_success',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", strconv.FormatInt(observedAt, 10)); err != nil {
			return err
		}
		if err := tx.Exec("DELETE FROM traffic_debits WHERE bucket<?", strconv.FormatInt(observedAt/60-90*1440, 10)); err != nil {
			return err
		}
		return tx.Exec("DELETE FROM traffic_debits WHERE rowid IN(SELECT rowid FROM traffic_debits ORDER BY bucket DESC LIMIT -1 OFFSET 65536)")
	})
}
func (s *Store) SavePackage(ctx context.Context, id, quota, expires, version int64, reset bool) error {
	if quota < 0 || quota > usage.MaxBytes || expires < 0 || expires > 253402300799 || version < 0 || version >= usage.MaxBytes {
		return usage.ErrInvalid
	}
	return s.DB.WithTx(ctx, func(tx *db.Tx) error {
		items, err := readPackages(tx)
		if err != nil {
			return err
		}
		p, ok := items[id]
		if !ok {
			return ErrNotFound
		}
		if p.Version != version {
			return ErrConflict
		}
		p.QuotaBytes = quota
		p.ExpiresAt = expires
		p.Version++
		if reset {
			p.Upload = 0
			p.Download = 0
			p.Period++
			p.PeriodStartedAt = time.Now().Unix()
			p.Incomplete = false
			p.NeedsReview = false
		}
		if err := savePackage(tx, p); err != nil {
			return err
		}
		action := "user.package.saved"
		if reset {
			action = "user.package.reset"
		}
		if err := tx.Exec("INSERT INTO events(time,action,details) VALUES(?,?,?)", strconv.FormatInt(time.Now().Unix(), 10), action, "user ID "+strconv.FormatInt(id, 10)); err != nil {
			return err
		}
		return tx.Exec("DELETE FROM events WHERE id NOT IN(SELECT id FROM events ORDER BY id DESC LIMIT 500)")
	})
}
func (s *Store) UserAccess(ctx context.Context, id int64) (bool, string, error) {
	allowed := false
	reason := ""
	err := s.DB.Read(ctx, func(tx *db.Tx) error {
		rows, err := tx.Query("SELECT users.id,users.enabled,users.created_at,user_packages.body,(SELECT value FROM settings WHERE key='xray_accounting_last_success') AS source_last FROM users LEFT JOIN user_packages ON users.id=user_packages.user_id WHERE users.id=?", strconv.FormatInt(id, 10))
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			return ErrNotFound
		}
		row := rows[0]
		if row["enabled"] != "1" {
			reason = "disabled"
			return nil
		}
		p, err := packageRow(row)
		if err != nil {
			return err
		}
		last, _ := strconv.ParseInt(row["source_last"], 10, 64)
		now := time.Now().Unix()
		fresh := last > 0 && now >= last && now-last <= usage.FreshFor
		reason = p.Reason(now, fresh)
		allowed = reason == ""
		return nil
	})
	return allowed, reason, err
}

type Debit struct {
	At       int64 `json:"at"`
	Period   int64 `json:"period"`
	Upload   int64 `json:"upload_bytes"`
	Download int64 `json:"download_bytes"`
}

func (s *Store) Debits(ctx context.Context, id int64) ([]Debit, error) {
	result := []Debit{}
	err := s.DB.Read(ctx, func(tx *db.Tx) error {
		rows, err := tx.Query("SELECT bucket,period,upload,download FROM traffic_debits WHERE user_id=? ORDER BY bucket DESC,period DESC LIMIT 60", strconv.FormatInt(id, 10))
		if err != nil {
			return err
		}
		for _, r := range rows {
			p, _ := strconv.ParseInt(r["period"], 10, 64)
			b, _ := strconv.ParseInt(r["bucket"], 10, 64)
			up, _ := strconv.ParseInt(r["upload"], 10, 64)
			down, _ := strconv.ParseInt(r["download"], 10, 64)
			result = append(result, Debit{b * 60, p, up, down})
		}
		return nil
	})
	return result, err
}

// Coverage guards prevent a finite local package from being bypassed by an
// engine or already deployed node without a trusted per-user accounting path.
var ErrUsageUnsupported = errors.New("traffic accounting is not available for every configured connection")

func (s *Store) CheckPackageCoverage(ctx context.Context, quota, expires int64) error {
	if quota == 0 && expires == 0 {
		return nil
	}
	return s.DB.Read(ctx, func(tx *db.Tx) error {
		rows, err := tx.Query("SELECT id FROM managed_nodes LIMIT 1")
		if err != nil {
			return err
		}
		if len(rows) > 0 {
			return ErrUsageUnsupported
		}
		if quota > 0 {
			rows, err = tx.Query("SELECT kind FROM protocols WHERE enabled=1 AND kind<>'vless-xhttp' LIMIT 1")
			if err != nil {
				return err
			}
			if len(rows) > 0 {
				return ErrUsageUnsupported
			}
		}
		return nil
	})
}
func (s *Store) HasRestrictedPackages(ctx context.Context, trafficOnly bool) (bool, error) {
	items, err := s.Packages(ctx)
	if err != nil {
		return false, err
	}
	for _, p := range items {
		if p.QuotaBytes > 0 || (!trafficOnly && p.ExpiresAt > 0) {
			return true, nil
		}
	}
	return false, nil
}
