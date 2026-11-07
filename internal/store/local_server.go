package store

import (
	"context"
	"errors"
	"net/netip"

	"github.com/arcvpn/arcvpn/internal/db"
)

// LocalServer is the identity of the local host, not a protocol listener.
// Its address is projected from existing explicit configurations, including
// disabled profiles. No dummy Reality configuration or secret is created.
type LocalServer struct {
	Address string `json:"address"`
}

var ErrLocalServerInUse = errors.New("last local server profile is required by existing users")

func (s *Store) LocalServer(ctx context.Context) (LocalServer, error) {
	var server LocalServer
	err := s.DB.Read(ctx, func(tx *db.Tx) error {
		rows, err := tx.Query("SELECT address FROM config UNION SELECT address FROM protocols")
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			return ErrNotFound
		}
		if len(rows) != 1 {
			return errors.New("conflicting local server addresses")
		}
		if _, err := netip.ParseAddr(rows[0]["address"]); err != nil {
			return errors.New("invalid local server address")
		}
		server.Address = rows[0]["address"]
		return nil
	})
	return server, err
}

// OptionalReality distinguishes an absent TCP listener from a database error.
func (s *Store) OptionalReality(ctx context.Context) (Reality, error) {
	r, err := s.ReadReality(ctx)
	if errors.Is(err, ErrNotFound) {
		return Reality{}, nil
	}
	return r, err
}

// DeleteReality removes only a disabled listener and preserves the local host identity needed by users.
func (s *Store) DeleteReality(ctx context.Context) error {
	return s.DB.WithTx(ctx, func(tx *db.Tx) error {
		rows, err := tx.Query("SELECT enabled FROM config WHERE id=1")
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			return ErrNotFound
		}
		if rows[0]["enabled"] == "1" {
			return ErrConflict
		}
		legacy, err := tx.Query("SELECT kind FROM protocols WHERE kind='amneziawg'")
		if err != nil {
			return err
		}
		if len(legacy) > 0 {
			return ErrConflict
		}
		remaining, err := tx.Query("SELECT COUNT(*) AS count FROM protocols")
		if err != nil {
			return err
		}
		users, err := tx.Query("SELECT id FROM users LIMIT 1")
		if err != nil {
			return err
		}
		if remaining[0]["count"] == "0" && len(users) > 0 {
			return ErrLocalServerInUse
		}
		return tx.Exec("DELETE FROM config WHERE id=1")
	})
}
