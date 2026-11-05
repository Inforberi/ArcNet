package store

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/arcvpn/arcvpn/internal/db"
	"github.com/arcvpn/arcvpn/internal/devices"
)

const devicesSchema = `CREATE TABLE IF NOT EXISTS user_devices(user_id INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,body TEXT NOT NULL)`
const deviceTokensSchema = `CREATE TABLE IF NOT EXISTS device_tokens(token TEXT PRIMARY KEY,user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,device_id TEXT NOT NULL)`

func readDevices(tx *db.Tx) (devices.State, error) {
	rows, err := tx.Query("SELECT body FROM user_devices ORDER BY user_id")
	if err != nil {
		return nil, err
	}
	result := devices.State{}
	for _, row := range rows {
		var r devices.Record
		if err := json.Unmarshal([]byte(row["body"]), &r); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, nil
}
func saveDevices(tx *db.Tx, r devices.Record) error {
	blob, err := json.Marshal(r)
	if err != nil {
		return err
	}
	if err := tx.Exec("INSERT INTO user_devices(user_id,body) VALUES(?,?) ON CONFLICT(user_id) DO UPDATE SET body=excluded.body", strconv.FormatInt(r.UserID, 10), string(blob)); err != nil {
		return err
	}
	if err := tx.Exec("DELETE FROM device_tokens WHERE user_id=?", strconv.FormatInt(r.UserID, 10)); err != nil {
		return err
	}
	for _, d := range r.Devices {
		if d.RevokedAt == 0 {
			if err := tx.Exec("INSERT INTO device_tokens(token,user_id,device_id) VALUES(?,?,?)", d.Token, strconv.FormatInt(r.UserID, 10), d.ID); err != nil {
				return err
			}
		}
	}
	return nil
}
func (s *Store) DeviceByToken(ctx context.Context, token string) (int64, string, error) {
	var id int64
	var deviceID string
	err := s.DB.Read(ctx, func(tx *db.Tx) error {
		rows, err := tx.Query("SELECT user_id,device_id FROM device_tokens WHERE token=?", token)
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			return ErrNotFound
		}
		id, _ = strconv.ParseInt(rows[0]["user_id"], 10, 64)
		deviceID = rows[0]["device_id"]
		return nil
	})
	return id, deviceID, err
}
func (s *Store) DeviceState(ctx context.Context) (devices.State, error) {
	var state devices.State
	err := s.DB.Read(ctx, func(tx *db.Tx) error { var err error; state, err = readDevices(tx); return err })
	return state, err
}
func (s *Store) UserDevices(ctx context.Context, id int64) (devices.Record, error) {
	var result devices.Record
	err := s.DB.Read(ctx, func(tx *db.Tx) error {
		rows, err := tx.Query("SELECT users.id,users.uuid,user_devices.body FROM users LEFT JOIN user_devices ON users.id=user_devices.user_id WHERE users.id=?", strconv.FormatInt(id, 10))
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			return ErrNotFound
		}
		result = devices.Record{UserID: id, Identity: rows[0]["uuid"], Devices: []devices.Device{}}
		if rows[0]["body"] != "" {
			return json.Unmarshal([]byte(rows[0]["body"]), &result)
		}
		return nil
	})
	return result, err
}
func (s *Store) HasManagedDevices(ctx context.Context) (bool, error) {
	var yes bool
	err := s.DB.Read(ctx, func(tx *db.Tx) error {
		rows, err := tx.Query("SELECT user_id FROM user_devices LIMIT 1")
		yes = len(rows) > 0
		return err
	})
	return yes, err
}

// ChangeDevices is a transaction-time CAS. An initial record permanently retires
// the common credential. Revocation is monotonic; slots count active credentials.
func (s *Store) ChangeDevices(ctx context.Context, id, version int64, change func(*devices.Record) error) error {
	return s.DB.WithTx(ctx, func(tx *db.Tx) error {
		state, err := readDevices(tx)
		if err != nil {
			return err
		}
		rows, err := tx.Query("SELECT id,uuid FROM users")
		if err != nil {
			return err
		}
		ids := map[int64]string{}
		for _, row := range rows {
			n, _ := strconv.ParseInt(row["id"], 10, 64)
			ids[n] = row["uuid"]
		}
		if ids[id] == "" {
			return ErrNotFound
		}
		index := -1
		r := devices.Record{UserID: id, Identity: ids[id], Devices: []devices.Device{}}
		for i, item := range state {
			if item.UserID == id {
				r = item
				index = i
				break
			}
		}
		if r.Version != version {
			return ErrConflict
		}
		if err := change(&r); err != nil {
			return err
		}
		r.Version++
		if index < 0 {
			state = append(state, r)
		} else {
			state[index] = r
		}
		if err := state.Validate(ids); err != nil {
			return err
		}
		// Independent of UI/app guards, prevent a policy with an unmanaged bypass.
		unsupported, err := tx.Query("SELECT kind FROM protocols WHERE enabled=1 AND kind<>'vless-xhttp' LIMIT 1")
		if err != nil {
			return err
		}
		nodes, err := tx.Query("SELECT id FROM managed_nodes LIMIT 1")
		if err != nil {
			return err
		}
		if len(unsupported) > 0 || len(nodes) > 0 {
			return ErrUsageUnsupported
		}
		return saveDevices(tx, r)
	})
}

// Preserve surviving identities on old backups. A stale backup cannot reactivate
// locally revoked credentials or restore the retired common credential.
func replaceDevices(tx *db.Tx, incoming *devices.State, before devices.State, oldIDs map[int64]string) error {
	rows, err := tx.Query("SELECT id,uuid FROM users")
	if err != nil {
		return err
	}
	ids := map[int64]string{}
	for _, row := range rows {
		id, _ := strconv.ParseInt(row["id"], 10, 64)
		ids[id] = row["uuid"]
	}
	next := devices.State{}
	if incoming != nil {
		next = append(next, (*incoming)...)
		if err := next.Validate(ids); err != nil {
			return err
		}
	}
	records := map[int64]devices.Record{}
	for _, r := range next {
		records[r.UserID] = r
	}
	for _, old := range before {
		if ids[old.UserID] != old.Identity || oldIDs[old.UserID] != old.Identity {
			continue
		}
		r, ok := records[old.UserID]
		if !ok {
			records[old.UserID] = old
			continue
		}
		byID := map[string]int{}
		for i, d := range r.Devices {
			byID[d.ID] = i
		}
		for _, d := range old.Devices {
			if i, ok := byID[d.ID]; ok {
				current := r.Devices[i]
				if current.UUID != d.UUID || current.Token != d.Token {
					return devices.ErrInvalid
				}
				if d.RevokedAt > current.RevokedAt {
					r.Devices[i].RevokedAt = d.RevokedAt
				}
			} else {
				r.Devices = append(r.Devices, d)
			}
		}
		// Current local policy is authoritative; importing is not a limit reset.
		r.Limit = old.Limit
		if old.Version > r.Version {
			r.Version = old.Version
		}
		r.Version++
		records[old.UserID] = r
	}
	next = devices.State{}
	for _, r := range records {
		next = append(next, r)
	}
	if err := next.Validate(ids); err != nil {
		return err
	}
	if len(next) > 0 {
		unsupported, err := tx.Query("SELECT kind FROM protocols WHERE enabled=1 AND kind<>'vless-xhttp' LIMIT 1")
		if err != nil {
			return err
		}
		nodes, err := tx.Query("SELECT id FROM managed_nodes LIMIT 1")
		if err != nil {
			return err
		}
		if len(unsupported) > 0 || len(nodes) > 0 {
			return ErrUsageUnsupported
		}
	}
	for _, r := range next {
		if err := saveDevices(tx, r); err != nil {
			return err
		}
	}
	return nil
}
