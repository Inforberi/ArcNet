// Package devices models individually revocable credentials, not physical HWID.
package devices

import (
	"errors"
	"regexp"
	"strings"
	"unicode/utf8"
)

const MaxActive = 100
const MaxHistory = 500
const MaxTotal = 10000

var ErrInvalid = errors.New("invalid device policy")
var ErrLimit = errors.New("device credential limit reached")
var uuid = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
var token = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)

type Device struct {
	WGPrivate string `json:"wg_private,omitempty"`
	WGPublic  string `json:"wg_public,omitempty"`
	WGAddress string `json:"wg_address,omitempty"`
	ID        string `json:"id"`
	Name      string `json:"name"`
	UUID      string `json:"uuid"`
	Token     string `json:"token"`
	CreatedAt int64  `json:"created_at"`
	RevokedAt int64  `json:"revoked_at"`
}
type Record struct {
	UserID   int64    `json:"user_id"`
	Identity string   `json:"identity"`
	Limit    int      `json:"limit"` // 0 means no product limit; bounded by MaxActive.
	Version  int64    `json:"version"`
	Devices  []Device `json:"devices"`
}
type State []Record

func ValidName(name string) bool {
	return strings.TrimSpace(name) == name && utf8.ValidString(name) && utf8.RuneCountInString(name) > 0 && utf8.RuneCountInString(name) <= 60 && !strings.ContainsAny(name, "\r\n\x00")
}
func (r Record) Active() int {
	n := 0
	for _, d := range r.Devices {
		if d.RevokedAt == 0 {
			n++
		}
	}
	return n
}
func (s State) Validate(identities map[int64]string) error {
	seen := map[int64]bool{}
	keys := map[string]bool{}
	total := 0
	for _, identity := range identities {
		keys[identity] = true
	}
	for _, r := range s {
		if r.UserID < 1 || seen[r.UserID] || identities[r.UserID] != r.Identity || !uuid.MatchString(r.Identity) || r.Limit < 0 || r.Limit > MaxActive || r.Version < 1 || r.Version >= 9_000_000_000_000_000 || len(r.Devices) > MaxHistory || r.Active() > MaxActive || (r.Limit > 0 && r.Active() > r.Limit) {
			return ErrInvalid
		}
		seen[r.UserID] = true
		for _, d := range r.Devices {
			if err := ValidateWireGuard(d); err != nil {
				return err
			}
			if d.WGPublic != "" {
				for _, k := range []string{d.WGPublic, d.WGAddress} {
					if keys[k] {
						return ErrInvalid
					}
					keys[k] = true
				}
			}
			if !uuid.MatchString(d.ID) || !uuid.MatchString(d.UUID) || !token.MatchString(d.Token) || !ValidName(d.Name) || d.CreatedAt < 1 || (d.RevokedAt != 0 && d.RevokedAt < d.CreatedAt) {
				return ErrInvalid
			}
			for _, k := range []string{d.ID, d.UUID, d.Token} {
				if keys[k] {
					return ErrInvalid
				}
				keys[k] = true
			}
		}
		total += len(r.Devices)
	}
	if total > MaxTotal {
		return ErrLimit
	}
	return nil
}
