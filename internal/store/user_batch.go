package store

import (
	"context"
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/arcvpn/arcvpn/internal/db"
	"github.com/arcvpn/arcvpn/internal/usage"
)

func validUserName(name string) bool {
	return utf8.ValidString(name) && strings.TrimSpace(name) == name && utf8.RuneCountInString(name) > 0 && utf8.RuneCountInString(name) <= 60 && strings.IndexFunc(name, unicode.IsControl) < 0
}

type UserBatchSelection struct {
	UserSelection
	PackageVersion int64 `json:"package_version"`
}
type UserBatchConnections struct {
	AllowedKinds []string `json:"allowed_kinds"`
}
type UserBatchChanges struct {
	QuotaBytes  *int64                `json:"quota_bytes"`
	ExpiresAt   *int64                `json:"expires_at"`
	Group       *string               `json:"group"`
	Connections *UserBatchConnections `json:"connections"`
}

func (in UserBatchChanges) Valid() bool {
	if in.QuotaBytes == nil && in.ExpiresAt == nil && in.Group == nil && in.Connections == nil {
		return false
	}
	if in.QuotaBytes != nil && (*in.QuotaBytes < 0 || *in.QuotaBytes > usage.MaxBytes) {
		return false
	}
	if in.ExpiresAt != nil && (*in.ExpiresAt < 0 || *in.ExpiresAt > 253402300799) {
		return false
	}
	p := UserPreferences{}
	if in.Group != nil {
		p.Group = *in.Group
	}
	if in.Connections != nil {
		p.AllowedKinds = in.Connections.AllowedKinds
	}
	return p.Valid()
}
func (s *Store) BulkEditUsers(ctx context.Context, items []UserBatchSelection, in UserBatchChanges) error {
	if len(items) < 1 || len(items) > 100 || !in.Valid() {
		return ErrInvalidUserPreferences
	}
	return s.DB.WithTx(ctx, func(tx *db.Tx) error {
		if err := requireNoCascadeLeases(tx); err != nil {
			return err
		}
		seen := map[int64]bool{}
		for _, item := range items {
			if item.ID < 1 || seen[item.ID] {
				return ErrInvalidUserPreferences
			}
			seen[item.ID] = true
			if err := checkUserSelection(tx, item.UserSelection); err != nil {
				return err
			}
			rows, err := tx.Query("SELECT name,preferences FROM users WHERE id=?", strconv.FormatInt(item.ID, 10))
			if err != nil {
				return err
			}
			var p UserPreferences
			if err = json.Unmarshal([]byte(rows[0]["preferences"]), &p); err != nil {
				return err
			}
			if in.Group != nil {
				p.Group = *in.Group
			}
			if in.Connections != nil {
				p.AllowedKinds = in.Connections.AllowedKinds
				if p.AllowedKinds != nil && !slices.Contains(p.AllowedKinds, p.DirectKind) {
					p.DirectKind = ""
					p.AccessMode = "subscription"
				}
			}
			if err = saveUserPreferences(tx, item.ID, rows[0]["name"], p, item.ExpectedVersion); err != nil {
				return err
			}
			if in.QuotaBytes != nil || in.ExpiresAt != nil {
				packages, err := readPackages(tx)
				if err != nil {
					return err
				}
				pack := packages[item.ID]
				if in.QuotaBytes != nil {
					pack.QuotaBytes = *in.QuotaBytes
				}
				if in.ExpiresAt != nil {
					pack.ExpiresAt = *in.ExpiresAt
				}
				if err = saveUserPackage(tx, item.ID, pack.QuotaBytes, pack.ExpiresAt, item.PackageVersion, false); err != nil {
					return err
				}
			}
		}
		if in.QuotaBytes != nil || in.ExpiresAt != nil || in.Connections != nil {
			return pendingUserRuntime(tx)
		}
		return nil
	})
}

// credentials is a pure factory: it must not call Store while the transaction is held.
func (s *Store) CreateUsers(ctx context.Context, users []User, credentials func(User) ([]ProtocolCredential, error)) ([]User, error) {
	if len(users) < 1 || len(users) > 100 {
		return nil, ErrInvalidUserPreferences
	}
	out := make([]User, 0, len(users))
	err := s.DB.WithTx(ctx, func(tx *db.Tx) error {
		all := []ProtocolCredential{}
		for _, u := range users {
			if !validUserName(u.Name) || !u.Preferences.Valid() {
				return ErrInvalidUserPreferences
			}
			if err := insertUser(tx, u); err != nil {
				return err
			}
			rows, err := tx.Query(userQuery+" WHERE token=?", u.Subscription)
			if err != nil {
				return err
			}
			u = userFromRow(rows[0])
			generated, err := credentials(u)
			if err != nil {
				return err
			}
			all = append(all, generated...)
			out = append(out, u)
		}
		if err := saveCredentials(tx, all); err != nil {
			return err
		}
		return pendingUserRuntime(tx)
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
