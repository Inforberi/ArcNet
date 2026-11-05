package protocols

import "github.com/arcvpn/arcvpn/internal/store"

type WireGuardPeer struct {
	UserID          int64
	Public, Address string
	Enabled         bool
}

// Includes retired aliases for accounting. Rendering uses Enabled only.
func WireGuardPeers(users []store.User, credentials []store.ProtocolCredential) []WireGuardPeer {
	legacy := map[int64]store.ProtocolCredential{}
	for _, c := range credentials {
		if c.Kind == WireGuard {
			legacy[c.UserID] = c
		}
	}
	result := []WireGuardPeer{}
	for _, u := range users {
		if c, ok := legacy[u.ID]; ok {
			result = append(result, WireGuardPeer{u.ID, c.PublicKey, c.Address, u.Enabled && u.DeviceAccess == nil})
		}
		if u.DeviceAccess != nil {
			for _, d := range u.DeviceAccess.Devices {
				if d.WGPublic != "" {
					result = append(result, WireGuardPeer{u.ID, d.WGPublic, d.WGAddress, u.Enabled && d.RevokedAt == 0})
				}
			}
		}
	}
	return result
}
