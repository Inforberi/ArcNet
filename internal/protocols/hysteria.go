package protocols

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/arcvpn/arcvpn/internal/store"
)

// HysteriaIdentity is shared by server rendering, client exports and accounting.
type HysteriaIdentity struct {
	UserID       int64
	Name, Secret string
	Enabled      bool
}

func HysteriaDeviceName(id string) string { return "d" + id }
func HysteriaIdentities(users []store.User, credentials []store.ProtocolCredential) []HysteriaIdentity {
	secrets := map[int64]string{}
	for _, c := range credentials {
		if c.Kind == Hysteria2 {
			secrets[c.UserID] = c.Secret
		}
	}
	result := []HysteriaIdentity{}
	for _, u := range users {
		// Legacy aliases remain known for the tail of the current process epoch.
		if secret := secrets[u.ID]; secret != "" {
			result = append(result, HysteriaIdentity{u.ID, fmt.Sprintf("u%d", u.ID), secret, u.Enabled && u.Allows(Hysteria2) && u.DeviceAccess == nil})
		}
		if u.DeviceAccess != nil {
			for _, d := range u.DeviceAccess.Devices {
				result = append(result, HysteriaIdentity{u.ID, HysteriaDeviceName(d.ID), d.UUID, u.Enabled && u.Allows(Hysteria2) && d.RevokedAt == 0})
			}
		}
	}
	return result
}
func renderHysteria(p store.Protocol, users []store.User, credentials []store.ProtocolCredential) ([]byte, error) {
	usersMap := map[string]string{}
	for _, id := range HysteriaIdentities(users, credentials) {
		if id.Enabled {
			if len(id.Secret) < 32 || strings.ContainsAny(id.Secret, "\r\n:\"'") {
				return nil, ErrInvalid
			}
			usersMap[id.Name] = id.Secret
		}
	}
	// JSON is a YAML-compatible document. The existing .yaml path remains stable,
	// while canonical JSON permits exact private config/identity verification.
	root := map[string]any{"listen": fmt.Sprintf("0.0.0.0:%d", p.Port), "tls": map[string]string{"cert": p.TLSCert, "key": p.TLSKey, "sniGuard": "dns-san"}, "auth": map[string]any{"type": "userpass", "userpass": usersMap}}
	if p.ServerOptions.UDPIdleSeconds != 0 {
		root["udpIdleTimeout"] = fmt.Sprintf("%ds", p.ServerOptions.UDPIdleSeconds)
	}
	if p.ServerOptions.QUICIdleSeconds != 0 {
		root["quic"] = map[string]any{"maxIdleTimeout": fmt.Sprintf("%ds", p.ServerOptions.QUICIdleSeconds)}
	}
	if p.ServerOptions.Salamander {
		root["obfs"] = map[string]any{"type": "salamander", "salamander": map[string]string{"password": p.ObfsKey}}
	}
	if p.ServerOptions.UpMbps != 0 || p.ServerOptions.DownMbps != 0 {
		root["bandwidth"] = map[string]string{"up": fmt.Sprintf("%d mbps", p.ServerOptions.UpMbps), "down": fmt.Sprintf("%d mbps", p.ServerOptions.DownMbps)}
	}
	return json.MarshalIndent(root, "", "  ")
}
func HysteriaLink(p store.Protocol, name, authID, secret string) string {
	q := url.Values{"sni": {p.SNI}, "insecure": {"0"}}
	if p.ServerOptions.Salamander {
		q.Set("obfs", "salamander")
		q.Set("obfs-password", p.ObfsKey)
	}
	return fmt.Sprintf("hysteria2://%s@%s/?%s#%s", url.PathEscape(authID+":"+secret), net.JoinHostPort(p.Address, strconv.Itoa(p.Port)), q.Encode(), url.QueryEscape(name+" Hysteria2"))
}
