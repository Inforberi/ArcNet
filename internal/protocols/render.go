package protocols

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/arcvpn/arcvpn/internal/store"
)

// Render emits config for a single engine, not a shell script. The caller must
// validate it with the actual installed engine and deploy it with rollback.
func Render(p store.Protocol, users []store.User, credentials []store.ProtocolCredential) ([]byte, error) {
	if err := Validate(p); err != nil {
		return nil, err
	}
	if p.Kind == XHTTP {
		return nil, errors.New("XHTTP is rendered inside Xray, alongside Reality TCP")
	}
	live := make(map[int64]bool, len(users))
	for _, u := range users {
		live[u.ID] = u.Enabled
	}
	creds := make([]store.ProtocolCredential, 0)
	for _, c := range credentials {
		if c.Kind == p.Kind && live[c.UserID] {
			creds = append(creds, c)
		}
	}
	sort.Slice(creds, func(i, j int) bool { return creds[i].UserID < creds[j].UserID })
	var out strings.Builder
	switch p.Kind {
	case Hysteria2:
		// Restrict interpolation to known, validated paths and base64url secrets.
		fmt.Fprintf(&out, "listen: 0.0.0.0:%d\ntls:\n  cert: %s\n  key: %s\n  sniGuard: dns-san\nauth:\n  type: userpass\n  userpass:\n", p.Port, p.TLSCert, p.TLSKey)
		if len(creds) == 0 {
			out.WriteString("    {}\n")
		} else {
			for _, c := range creds {
				if len(c.Secret) < 32 || strings.ContainsAny(c.Secret, "\r\n:\"'") {
					return nil, ErrInvalid
				}
				fmt.Fprintf(&out, "    u%d: %s\n", c.UserID, c.Secret)
			}
		}
	case WireGuard, AmneziaWG:
		address := "10.76.0.1/16"
		if p.Kind == AmneziaWG {
			address = "10.77.0.1/16"
		}
		fmt.Fprintf(&out, "[Interface]\nAddress = %s\nListenPort = %d\nPrivateKey = %s\n", address, p.Port, p.PrivateKey)
		if p.Kind == AmneziaWG {
			out.WriteString(awgSettings(p.ObfsKey))
		}
		for _, c := range creds {
			ip, err := AddressForUser(p.Kind, c.UserID)
			if err != nil || c.Address != ip || !validWGKey(c.PublicKey) {
				return nil, ErrInvalid
			}
			fmt.Fprintf(&out, "\n[Peer]\nPublicKey = %s\nAllowedIPs = %s/32\n", c.PublicKey, c.Address)
		}
	default:
		return nil, ErrInvalid
	}
	return []byte(out.String()), nil
}
