package store

import (
	"context"

	"github.com/arcvpn/arcvpn/internal/db"
)

// RuntimePlan contains no secrets or executable input. A single SQLite read
// observes all enabled flags before a root CLI chooses fixed local services.
type RuntimePlan struct {
	Xray          bool `json:"xray"`
	Hysteria      bool `json:"hysteria"`
	WireGuard     bool `json:"wireguard"`
	LegacyAmnezia bool `json:"legacy_amnezia"`
}

func (s *Store) RuntimePlan(ctx context.Context) (RuntimePlan, error) {
	var plan RuntimePlan
	err := s.DB.Read(ctx, func(tx *db.Tx) error {
		rows, err := tx.Query("SELECT 'vless-reality' AS kind,enabled FROM config UNION ALL SELECT kind,enabled FROM protocols")
		if err != nil {
			return err
		}
		for _, row := range rows {
			if row["enabled"] != "1" {
				continue
			}
			switch row["kind"] {
			case "vless-reality", "vless-xhttp":
				plan.Xray = true
			case "hysteria2":
				plan.Hysteria = true
			case "wireguard":
				plan.WireGuard = true
			case "amneziawg":
				plan.LegacyAmnezia = true
			}
		}
		return nil
	})
	return plan, err
}
