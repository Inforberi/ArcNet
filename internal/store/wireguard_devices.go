package store

import (
	"context"
	"fmt"

	"github.com/arcvpn/arcvpn/internal/db"
	"github.com/arcvpn/arcvpn/internal/devices"
)

func wireGuardClaims(tx *db.Tx, state devices.State) (map[string]bool, error) {
	used := map[string]bool{}
	rows, err := tx.Query("SELECT address FROM protocol_credentials WHERE kind='wireguard'")
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		if !devices.WireGuardAddress(row["address"]) || used[row["address"]] {
			return nil, devices.ErrInvalid
		}
		used[row["address"]] = true
	}
	for _, r := range state {
		for _, d := range r.Devices {
			if d.WGAddress != "" {
				if !devices.WireGuardAddress(d.WGAddress) || used[d.WGAddress] {
					return nil, devices.ErrInvalid
				}
				used[d.WGAddress] = true
			}
		}
	}
	return used, nil
}
func freeWireGuardAddress(used map[string]bool) (string, error) {
	for n := 65534; n >= 2; n-- {
		address := fmt.Sprintf("10.76.%d.%d", n>>8, n&255)
		if !used[address] {
			used[address] = true
			return address, nil
		}
	}
	return "", devices.ErrLimit
}

// Same managed /16, no new routes or firewall rules. Legacy keys/IPs are kept;
// new peers reserve a free address transactionally, including revoked history.
func ensureWireGuardDevices(tx *db.Tx, state devices.State) (map[int64]bool, error) {
	changed := map[int64]bool{}
	rows, err := tx.Query("SELECT kind FROM protocols WHERE kind='wireguard' LIMIT 1")
	if err != nil || len(rows) == 0 {
		return changed, err
	}
	used, err := wireGuardClaims(tx, state)
	if err != nil {
		return nil, err
	}
	for i := range state {
		for j := range state[i].Devices {
			d := &state[i].Devices[j]
			if err := devices.ValidateWireGuard(*d); err != nil {
				return nil, err
			}
			if d.WGPrivate != "" {
				continue
			}
			d.WGPrivate, d.WGPublic, err = devices.NewWireGuardKeys()
			if err != nil {
				return nil, err
			}
			d.WGAddress, err = freeWireGuardAddress(used)
			if err != nil {
				return nil, err
			}
			changed[state[i].UserID] = true
		}
	}
	return changed, nil
}
func (s *Store) EnsureWireGuardDevices(ctx context.Context) error {
	return s.DB.WithTx(ctx, func(tx *db.Tx) error {
		state, err := readDevices(tx)
		if err != nil {
			return err
		}
		changed, err := ensureWireGuardDevices(tx, state)
		if err != nil {
			return err
		}
		for _, r := range state {
			if changed[r.UserID] {
				r.Version++
				if err := saveDevices(tx, r); err != nil {
					return err
				}
			}
		}
		return nil
	})
}
