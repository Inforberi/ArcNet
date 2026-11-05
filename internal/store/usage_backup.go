package store

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/arcvpn/arcvpn/internal/db"
	"github.com/arcvpn/arcvpn/internal/devices"
	"github.com/arcvpn/arcvpn/internal/usage"
)

func readUsageState(tx *db.Tx) (usage.State, error) {
	state := usage.State{Packages: []usage.Package{}, Checkpoints: []usage.Counter{}, Debits: []usage.Debit{}}
	packages, err := readPackages(tx)
	if err != nil {
		return state, err
	}
	for _, p := range packages {
		state.Packages = append(state.Packages, p)
	}
	rows, err := tx.Query("SELECT body FROM traffic_checkpoints ORDER BY user_id,stream")
	if err != nil {
		return state, err
	}
	for _, row := range rows {
		var c usage.Counter
		if json.Unmarshal([]byte(row["body"]), &c) != nil {
			return state, usage.ErrInvalid
		}
		state.Checkpoints = append(state.Checkpoints, c)
	}
	rows, err = tx.Query("SELECT user_id,period,bucket,upload,download FROM traffic_debits ORDER BY bucket DESC,user_id,period")
	if err != nil {
		return state, err
	}
	for _, row := range rows {
		parse := func(k string) int64 { n, _ := strconv.ParseInt(row[k], 10, 64); return n }
		state.Debits = append(state.Debits, usage.Debit{UserID: parse("user_id"), Period: parse("period"), At: parse("bucket") * 60, Upload: parse("upload"), Download: parse("download")})
	}
	return state, nil
}
func (s *Store) UsageBackup(ctx context.Context) (usage.State, error) {
	var state usage.State
	err := s.DB.Read(ctx, func(tx *db.Tx) error { var err error; state, err = readUsageState(tx); return err })
	return state, err
}

// RestoreDeletedUser is the compensation for a failed engine apply. Credentials,
// package totals, checkpoints and debit history are restored atomically.
func (s *Store) RestoreDeletedUser(ctx context.Context, u User, credentials []ProtocolCredential, accounting usage.State, deviceState ...devices.Record) error {
	return s.DB.WithTx(ctx, func(tx *db.Tx) error {
		if err := tx.Exec("INSERT INTO users(id,name,uuid,token,enabled,created_at) VALUES(?,?,?,?,?,?)", strconv.FormatInt(u.ID, 10), u.Name, u.UUID, u.Subscription, boolean(u.Enabled), strconv.FormatInt(u.CreatedAt, 10)); err != nil {
			return err
		}
		for _, c := range credentials {
			if c.UserID != u.ID {
				return usage.ErrInvalid
			}
			if err := tx.Exec("INSERT INTO protocol_credentials(user_id,kind,secret,private_key,public_key,address) VALUES(?,?,?,?,?,?)", strconv.FormatInt(c.UserID, 10), c.Kind, c.Secret, c.PrivateKey, c.PublicKey, c.Address); err != nil {
				return err
			}
		}
		for _, p := range accounting.Packages {
			if p.UserID == u.ID {
				if err := savePackage(tx, p); err != nil {
					return err
				}
			}
		}
		for _, c := range accounting.Checkpoints {
			if c.UserID == u.ID {
				body, _ := json.Marshal(c)
				if err := tx.Exec("INSERT INTO traffic_checkpoints(user_id,stream,body) VALUES(?,?,?)", strconv.FormatInt(c.UserID, 10), c.Stream, string(body)); err != nil {
					return err
				}
			}
		}
		for _, d := range accounting.Debits {
			if d.UserID == u.ID {
				if err := tx.Exec("INSERT INTO traffic_debits(user_id,period,bucket,upload,download) VALUES(?,?,?,?,?)", strconv.FormatInt(d.UserID, 10), strconv.FormatInt(d.Period, 10), strconv.FormatInt(d.At/60, 10), strconv.FormatInt(d.Upload, 10), strconv.FormatInt(d.Download, 10)); err != nil {
					return err
				}
			}
		}
		for _, r := range deviceState {
			if r.UserID != u.ID || r.Identity != u.UUID {
				return devices.ErrInvalid
			}
			if r.Version > 0 {
				if err := saveDevices(tx, r); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// A portable restore is not a quota reset. For the same user credential, keep
// the later period and the high-water totals/checkpoints already observed here.
// Legacy backups preserve surviving users' accounting instead of clearing it.
func replaceUsageState(tx *db.Tx, incoming *usage.State, before usage.State, oldIdentities map[int64]string) error {
	rows, err := tx.Query("SELECT id,uuid FROM users")
	if err != nil {
		return err
	}
	identities := map[int64]string{}
	for _, r := range rows {
		id, _ := strconv.ParseInt(r["id"], 10, 64)
		identities[id] = r["uuid"]
	}
	state := usage.State{}
	if incoming != nil {
		state = *incoming
		if state.Validate(identities) != nil {
			return usage.ErrInvalid
		}
	}
	packages := map[int64]usage.Package{}
	for _, p := range state.Packages {
		packages[p.UserID] = p
	}
	for _, old := range before.Packages {
		if identities[old.UserID] == "" || identities[old.UserID] != oldIdentities[old.UserID] {
			continue
		}
		p, ok := packages[old.UserID]
		if !ok {
			packages[old.UserID] = old
			continue
		}
		if old.Period > p.Period {
			p.Period = old.Period
			p.PeriodStartedAt = old.PeriodStartedAt
			p.Upload = old.Upload
			p.Download = old.Download
		}
		if old.Period == p.Period {
			p.Incomplete = p.Incomplete || old.Incomplete
			p.NeedsReview = p.NeedsReview || old.NeedsReview
			if p.Upload < old.Upload {
				p.Upload = old.Upload
			}
			if p.Download < old.Download {
				p.Download = old.Download
			}
		}
		if old.ObservedAt > p.ObservedAt {
			p.ObservedAt = old.ObservedAt
		}
		if old.Version >= p.Version {
			p.Version = old.Version + 1
		}
		packages[p.UserID] = p
	}
	for _, p := range packages {
		if err := savePackage(tx, p); err != nil {
			return err
		}
		if p.QuotaBytes > 0 {
			unsupported, err := tx.Query("SELECT kind FROM protocols WHERE enabled=1 AND kind<>'vless-xhttp' LIMIT 1")
			if err != nil {
				return err
			}
			if len(unsupported) > 0 {
				return ErrUsageUnsupported
			}
		}
		if p.QuotaBytes > 0 || p.ExpiresAt > 0 {
			nodes, err := tx.Query("SELECT id FROM managed_nodes LIMIT 1")
			if err != nil {
				return err
			}
			if len(nodes) > 0 {
				return ErrUsageUnsupported
			}
		}
	}
	key := func(c usage.Counter) string { return strconv.FormatInt(c.UserID, 10) + "/" + c.Stream }
	counters := map[string]usage.Counter{}
	for _, c := range state.Checkpoints {
		counters[key(c)] = c
	}
	for _, c := range before.Checkpoints {
		if identities[c.UserID] != c.Identity || oldIdentities[c.UserID] != c.Identity {
			continue
		}
		current, ok := counters[key(c)]
		if !ok || c.ObservedAt > current.ObservedAt {
			counters[key(c)] = c
		}
	}
	for _, c := range counters {
		body, _ := json.Marshal(c)
		if err := tx.Exec("INSERT INTO traffic_checkpoints(user_id,stream,body) VALUES(?,?,?)", strconv.FormatInt(c.UserID, 10), c.Stream, string(body)); err != nil {
			return err
		}
	}
	debits := map[string]usage.Debit{}
	debitKey := func(d usage.Debit) string {
		return strconv.FormatInt(d.UserID, 10) + "/" + strconv.FormatInt(d.Period, 10) + "/" + strconv.FormatInt(d.At/60, 10)
	}
	for _, d := range state.Debits {
		debits[debitKey(d)] = d
	}
	for _, d := range before.Debits {
		if identities[d.UserID] == "" || identities[d.UserID] != oldIdentities[d.UserID] {
			continue
		}
		k := debitKey(d)
		current, ok := debits[k]
		if !ok {
			debits[k] = d
		} else {
			if current.Upload < d.Upload {
				current.Upload = d.Upload
			}
			if current.Download < d.Download {
				current.Download = d.Download
			}
			debits[k] = current
		}
	}
	for _, d := range debits {
		if err := tx.Exec("INSERT INTO traffic_debits(user_id,period,bucket,upload,download) VALUES(?,?,?,?,?)", strconv.FormatInt(d.UserID, 10), strconv.FormatInt(d.Period, 10), strconv.FormatInt(d.At/60, 10), strconv.FormatInt(d.Upload, 10), strconv.FormatInt(d.Download, 10)); err != nil {
			return err
		}
	}
	if err := tx.Exec("DELETE FROM traffic_debits WHERE rowid IN(SELECT rowid FROM traffic_debits ORDER BY bucket DESC LIMIT -1 OFFSET 65536)"); err != nil {
		return err
	}
	if err := tx.Exec("DELETE FROM settings WHERE key='xray_accounting_last_success'"); err != nil {
		return err
	}
	return tx.Exec("INSERT INTO settings(key,value) VALUES('usage_runtime','unverified') ON CONFLICT(key) DO UPDATE SET value=excluded.value")
}
