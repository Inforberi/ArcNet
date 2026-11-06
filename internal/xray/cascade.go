package xray

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/netip"
	"reflect"
	"regexp"

	"github.com/arcvpn/arcvpn/internal/auth"
	"github.com/arcvpn/arcvpn/internal/nodebootstrap"
	"github.com/arcvpn/arcvpn/internal/store"
)

// CascadeChannel is a separate machine identity, never a user/enrollment key.
// Callers must persist it encrypted before deployment. Never log config bytes.
type CascadeChannel struct {
	ID      string `json:"-"`
	Private string `json:"-"`
	Public  string `json:"-"`
	ShortID string `json:"-"`
	Port    int    `json:"-"`
}

func (CascadeChannel) String() string { return "[redacted cascade channel]" }

func NewCascadeChannel(port int) (CascadeChannel, error) {
	if port < 1024 || port > 65535 {
		return CascadeChannel{}, errors.New("invalid inter-node port")
	}
	key, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return CascadeChannel{}, err
	}
	id, err := auth.UUID()
	if err != nil {
		return CascadeChannel{}, err
	}
	sid := make([]byte, 8)
	if _, err = rand.Read(sid); err != nil {
		return CascadeChannel{}, err
	}
	return CascadeChannel{ID: id, Private: base64.RawURLEncoding.EncodeToString(key.Bytes()), Public: base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes()), ShortID: hex.EncodeToString(sid), Port: port}, nil
}

const CascadeInbound = "arcvpn-hop-in"
const CascadeOutbound = "arcvpn-hop-out"

// GenerateCascade produces a dedicated, fail-closed TCP/REALITY A->B pair.
// It preserves both nodes' existing client credentials, adding a separate
// listener on B. No fallback, endpoint publication or OS/firewall change occurs.
func GenerateCascade(entry, exit store.Reality, entryUsers, exitUsers []store.User, channel CascadeChannel) (entryConfig, exitConfig []byte, err error) {
	if !entry.Enabled || !exit.Enabled {
		err = errors.New("both cascade profiles must be enabled")
		return
	}
	if err = Validate(entry); err != nil {
		return
	}
	if err = Validate(exit); err != nil {
		return
	}
	entryIP, _ := netip.ParseAddr(entry.Address)
	exitIP, _ := netip.ParseAddr(exit.Address)
	if !nodebootstrap.PublicIP(entryIP) || !nodebootstrap.PublicIP(exitIP) || entryIP.Unmap() == exitIP.Unmap() {
		err = errors.New("cascade requires distinct public node addresses")
		return
	}
	if channel.Port == exit.Port || channel.Port < 1024 || channel.Port > 65535 || !cascadeUUID.MatchString(channel.ID) {
		err = errors.New("invalid or conflicting channel listener")
		return
	}
	private, e := base64.RawURLEncoding.DecodeString(channel.Private)
	if e != nil {
		err = errors.New("invalid channel key")
		return
	}
	key, e := ecdh.X25519().NewPrivateKey(private)
	if e != nil || base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes()) != channel.Public || !hexID.MatchString(channel.ShortID) {
		err = errors.New("channel keypair mismatch")
		return
	}
	for _, users := range [][]store.User{entryUsers, exitUsers} {
		for _, u := range users {
			if u.UUID == channel.ID {
				err = errors.New("inter-node identity must not be a user identity")
				return
			}
			if u.DeviceAccess == nil {
				continue
			}
			for _, d := range u.DeviceAccess.Devices {
				if d.UUID == channel.ID {
					err = errors.New("inter-node identity must not be a device identity")
					return
				}
			}
		}
	}
	if channel.Private == entry.Private || channel.Private == exit.Private {
		err = errors.New("inter-node key must be independent")
		return
	}
	entryConfig, err = Generate(entry, entryUsers)
	if err != nil {
		return
	}
	exitConfig, err = Generate(exit, exitUsers)
	if err != nil {
		return
	}
	hop := exit
	hop.Port, hop.Private, hop.Public, hop.ShortID = channel.Port, channel.Private, channel.Public, channel.ShortID
	raw, e := Generate(hop, []store.User{{Name: "inter-node", UUID: channel.ID, Enabled: true}})
	if e != nil {
		err = e
		return
	}
	var a, b, h map[string]any
	_ = json.Unmarshal(entryConfig, &a)
	_ = json.Unmarshal(exitConfig, &b)
	_ = json.Unmarshal(raw, &h)
	listener := h["inbounds"].([]any)[0].(map[string]any)
	listener["tag"] = CascadeInbound
	b["inbounds"] = append(b["inbounds"].([]any), listener)
	out := cascadeOutbound(exit.Address, channel.Port, channel.ID, exit.SNI, channel.Public, channel.ShortID)
	a["outbounds"] = []any{map[string]any{"protocol": "blackhole", "tag": "block"}, out}
	a["routing"] = cascadeRouting(false)
	b["outbounds"] = []any{map[string]any{"protocol": "blackhole", "tag": "block"}, cascadeExitOutbound()}
	b["routing"] = cascadeRouting(true)
	entryConfig, err = json.Marshal(a)
	if err == nil {
		exitConfig, err = json.Marshal(b)
	}
	return
}

var cascadeUUID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func cascadeOutbound(address string, port int, id, sni, public, short string) map[string]any {
	return map[string]any{"protocol": "vless", "tag": CascadeOutbound,
		"settings":       map[string]any{"address": address, "port": port, "id": id, "encryption": "none", "flow": "xtls-rprx-vision"},
		"streamSettings": map[string]any{"network": "tcp", "security": "reality", "realitySettings": map[string]any{"serverName": sni, "fingerprint": "chrome", "password": public, "shortId": short}}}
}
func cascadeExitOutbound() map[string]any {
	return map[string]any{"protocol": "freedom", "tag": "direct", "settings": map[string]any{"targetStrategy": "UseIP", "finalRules": []any{map[string]any{"action": "block", "ip": cascadeBlockedIPs()}, map[string]any{"action": "allow"}}}}
}
func cascadeBlockedIPs() []string {
	return []string{"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8", "169.254.0.0/16", "172.16.0.0/12", "192.0.0.0/24", "192.0.2.0/24", "192.168.0.0/16", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "224.0.0.0/4", "240.0.0.0/4", "::/96", "2001:db8::/32", "fc00::/7", "fe80::/10", "ff00::/8"}
}
func cascadeRouting(exit bool) map[string]any {
	tags := []string{"vless-reality"}
	target := CascadeOutbound
	if exit {
		tags = append(tags, CascadeInbound)
		target = "direct"
	}
	return map[string]any{"domainStrategy": "AsIs", "rules": []any{
		map[string]any{"type": "field", "inboundTag": tags, "ip": cascadeBlockedIPs(), "outboundTag": "block"},
		map[string]any{"type": "field", "inboundTag": tags, "outboundTag": target},
	}}
}

// ValidateCascadeForwarding validates only the fixed forwarding shape. The
// privileged caller MUST also validate every listener with its normal allowlist.
// Unknown routes/outbounds, local destinations and direct fallback are rejected.
func ValidateCascadeForwarding(raw []byte) error {
	var config struct {
		Inbounds []struct {
			Tag  string `json:"tag"`
			Port int    `json:"port"`
		} `json:"inbounds"`
		Outbounds []json.RawMessage `json:"outbounds"`
		Routing   json.RawMessage   `json:"routing"`
	}
	if json.Unmarshal(raw, &config) != nil {
		return errors.New("invalid cascade config")
	}
	exit := len(config.Inbounds) == 2
	if len(config.Inbounds) < 1 || len(config.Inbounds) > 2 || config.Inbounds[0].Tag != "vless-reality" {
		return errors.New("invalid cascade listeners")
	}
	if exit && (config.Inbounds[1].Tag != CascadeInbound || config.Inbounds[1].Port < 1024 || config.Inbounds[0].Port == config.Inbounds[1].Port) {
		return errors.New("invalid channel listener")
	}
	if !sameJSON(config.Routing, cascadeRouting(exit)) || len(config.Outbounds) != 2 || !sameJSON(config.Outbounds[0], map[string]any{"protocol": "blackhole", "tag": "block"}) {
		return errors.New("cascade must use fixed fail-closed forwarding")
	}
	if exit {
		if !sameJSON(config.Outbounds[1], cascadeExitOutbound()) {
			return errors.New("invalid exit outbound")
		}
		return nil
	}
	var out struct {
		Settings struct {
			Address string `json:"address"`
			Port    int    `json:"port"`
			ID      string `json:"id"`
		} `json:"settings"`
		Stream struct {
			Reality struct {
				SNI    string `json:"serverName"`
				Public string `json:"password"`
				Short  string `json:"shortId"`
			} `json:"realitySettings"`
		} `json:"streamSettings"`
	}
	if json.Unmarshal(config.Outbounds[1], &out) != nil {
		return errors.New("invalid channel outbound")
	}
	ip, e := netip.ParseAddr(out.Settings.Address)
	key, e2 := base64.RawURLEncoding.DecodeString(out.Stream.Reality.Public)
	if e != nil || !nodebootstrap.PublicIP(ip) || out.Settings.Port < 1024 || out.Settings.Port > 65535 || !cascadeUUID.MatchString(out.Settings.ID) || e2 != nil || len(key) != 32 || !sniPattern.MatchString(out.Stream.Reality.SNI) || !hexID.MatchString(out.Stream.Reality.Short) {
		return errors.New("invalid channel destination or credential")
	}
	expected := cascadeOutbound(out.Settings.Address, out.Settings.Port, out.Settings.ID, out.Stream.Reality.SNI, out.Stream.Reality.Public, out.Stream.Reality.Short)
	if !sameJSON(config.Outbounds[1], expected) {
		return errors.New("unsupported inter-node outbound fields")
	}
	return nil
}
func sameJSON(raw []byte, value any) bool {
	var actual, expected any
	data, _ := json.Marshal(value)
	return json.Unmarshal(raw, &actual) == nil && json.Unmarshal(data, &expected) == nil && reflect.DeepEqual(actual, expected)
}
