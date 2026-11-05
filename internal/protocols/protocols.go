// Package protocols owns independent, local VPN protocol configurations and
// client exports. It does not execute arbitrary commands or configure the OS.
package protocols

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/arcvpn/arcvpn/internal/auth"
	"github.com/arcvpn/arcvpn/internal/devices"
	"github.com/arcvpn/arcvpn/internal/store"
	"github.com/arcvpn/arcvpn/internal/xray"
)

const (
	XHTTP     = "vless-xhttp"
	Hysteria2 = "hysteria2"
	WireGuard = "wireguard"
	AmneziaWG = "amneziawg"
)

var hostnameRE = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9.-]{0,251}[A-Za-z0-9])?$`)
var pathRE = regexp.MustCompile(`^/[a-zA-Z0-9/_-]{1,100}$`)
var base64KeyRE = regexp.MustCompile(`^[A-Za-z0-9+/]{42}[A-Za-z0-9+/=]{2}$`)
var ErrInvalid = errors.New("invalid protocol configuration")

func Kinds() []string { return []string{XHTTP, Hysteria2, WireGuard, AmneziaWG} }
func IsSupported(kind string) bool {
	for _, v := range Kinds() {
		if v == kind {
			return true
		}
	}
	return false
}
func DefaultPort(kind string) int {
	switch kind {
	case XHTTP:
		return 2087
	case Hysteria2:
		return 8444
	case WireGuard:
		return 51820
	case AmneziaWG:
		return 51821
	}
	return 0
}
func Transport(kind string) string {
	if kind == WireGuard || kind == AmneziaWG || kind == Hysteria2 {
		return "udp"
	}
	return "tcp"
}
func validateHostname(host string) bool {
	if ip, err := netip.ParseAddr(host); err == nil {
		return ip.IsValid()
	}
	return len(host) <= 253 && len(host) >= 3 && hostnameRE.MatchString(host) && !strings.Contains(host, "..") && strings.Contains(host, ".")
}
func Validate(p store.Protocol) error {
	if !IsSupported(p.Kind) || p.Port < 1 || p.Port > 65535 {
		return fmt.Errorf("%w: kind or port", ErrInvalid)
	}
	if _, err := netip.ParseAddr(p.Address); err != nil {
		return fmt.Errorf("%w: address must be an IP", ErrInvalid)
	}
	switch p.Kind {
	case XHTTP:
		if !validateHostname(p.SNI) || !validateHostname(p.Target) || !pathRE.MatchString(p.Path) || len(p.ShortID) != 16 {
			return fmt.Errorf("%w: XHTTP target/SNI/path", ErrInvalid)
		}
		if len(p.PrivateKey) < 40 || len(p.PublicKey) < 40 {
			return fmt.Errorf("%w: Reality keys", ErrInvalid)
		}
		if _, err := hex.DecodeString(p.ShortID); err != nil {
			return fmt.Errorf("%w: short ID", ErrInvalid)
		}
	case Hysteria2:
		if p.SNI != p.Address {
			return fmt.Errorf("%w: TLS IP certificate only covers configured public IP", ErrInvalid)
		}
		if !validateHostname(p.SNI) {
			return fmt.Errorf("%w: Hysteria certificate name", ErrInvalid)
		}
		if p.TLSCert != "/etc/arcvpn/tls/fullchain.pem" || p.TLSKey != "/etc/arcvpn/tls/privkey.pem" {
			return fmt.Errorf("%w: untrusted TLS path", ErrInvalid)
		}
	case WireGuard, AmneziaWG:
		if !validWGKey(p.PrivateKey) || !validWGKey(p.PublicKey) {
			return fmt.Errorf("%w: WireGuard keys", ErrInvalid)
		}
		if p.Kind == AmneziaWG && !validWGKey(p.ObfsKey) {
			return fmt.Errorf("%w: AmneziaWG header key", ErrInvalid)
		}
	}
	return nil
}
func validWGKey(v string) bool {
	if !base64KeyRE.MatchString(v) {
		return false
	}
	b, e := base64.StdEncoding.DecodeString(v)
	return e == nil && len(b) == 32
}

func WGKeys() (private, public string, err error) {
	var seed [32]byte
	if _, err = rand.Read(seed[:]); err != nil {
		return "", "", err
	}
	// Mirror the scalar clamping performed by wg genkey.
	seed[0] &= 248
	seed[31] &= 127
	seed[31] |= 64
	privateKey, err := ecdh.X25519().NewPrivateKey(seed[:])
	if err != nil {
		return "", "", err
	}
	return base64.StdEncoding.EncodeToString(seed[:]), base64.StdEncoding.EncodeToString(privateKey.PublicKey().Bytes()), nil
}
func secretBytes(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

// New creates one independently keyed listener; nothing is persisted or opened
// until the application confirms and successfully applies its configuration.
func New(ctx context.Context, kind, address string, port int, sni, target, xrayBinary string) (store.Protocol, error) {
	if port == 0 {
		port = DefaultPort(kind)
	}
	p := store.Protocol{Kind: kind, Address: strings.TrimSpace(address), Port: port, SNI: strings.TrimSpace(sni), Target: strings.TrimSpace(target), Enabled: true}
	if !IsSupported(kind) {
		return p, ErrInvalid
	}
	switch kind {
	case XHTTP:
		if p.Target == "" {
			p.Target = p.SNI
		}
		if p.SNI == "" {
			p.SNI = p.Target
		}
		priv, pub, sid, err := xray.GenerateKeys(ctx, xrayBinary)
		if err != nil {
			return p, err
		}
		path, err := auth.Secret(18)
		if err != nil {
			return p, err
		}
		p.PrivateKey, p.PublicKey, p.ShortID, p.Path = priv, pub, sid, "/"+path
	case Hysteria2:
		if p.SNI == "" {
			p.SNI = p.Address
		}
		p.TLSCert = "/etc/arcvpn/tls/fullchain.pem"
		p.TLSKey = "/etc/arcvpn/tls/privkey.pem"
	case WireGuard, AmneziaWG:
		priv, pub, err := WGKeys()
		if err != nil {
			return p, err
		}
		p.PrivateKey, p.PublicKey = priv, pub
		if kind == AmneziaWG {
			p.ObfsKey, err = secretBytes(32)
			if err != nil {
				return p, err
			}
		}
	}
	return p, Validate(p)
}

func AddressForUser(kind string, userID int64) (string, error) {
	if kind != WireGuard && kind != AmneziaWG {
		return "", nil
	}
	if userID <= 0 || userID > 65000 {
		return "", errors.New("tunnel address capacity exceeded")
	}
	prefix := "10.76"
	if kind == AmneziaWG {
		prefix = "10.77"
	}
	n := userID + 1
	return fmt.Sprintf("%s.%d.%d", prefix, n>>8, n&255), nil
}
func NewCredential(kind string, user store.User) (store.ProtocolCredential, error) {
	c := store.ProtocolCredential{UserID: user.ID, Kind: kind}
	if user.ID <= 0 {
		return c, ErrInvalid
	}
	switch kind {
	case XHTTP:
		c.Secret = user.UUID
	case Hysteria2:
		v, err := auth.Secret(32)
		if err != nil {
			return c, err
		}
		c.Secret = v
	case WireGuard, AmneziaWG:
		var err error
		c.PrivateKey, c.PublicKey, err = WGKeys()
		if err != nil {
			return c, err
		}
		c.Address, err = AddressForUser(kind, user.ID)
		if err != nil {
			return c, err
		}
	default:
		return c, ErrInvalid
	}
	return c, nil
}

// Connection is explicitly secret-bearing; only return it to a valid admin
// session or the correct subscription token.
type Connection struct {
	Kind          string `json:"kind"`
	Label         string `json:"label"`
	Format        string `json:"format"`
	Link          string `json:"link,omitempty"`
	Configuration string `json:"configuration,omitempty"`
}

func Connections(user store.User, profiles []store.Protocol, credentials []store.ProtocolCredential) ([]Connection, error) {
	if !user.Enabled {
		return []Connection{}, nil
	}
	creds := make(map[string]store.ProtocolCredential, len(credentials))
	for _, c := range credentials {
		if c.UserID == user.ID {
			creds[c.Kind] = c
		}
	}
	out := make([]Connection, 0, len(profiles)+1)
	for _, p := range profiles {
		if !p.Enabled {
			continue
		}
		c, ok := creds[p.Kind]
		if !ok {
			continue
		}
		if err := Validate(p); err != nil {
			return nil, err
		}
		switch p.Kind {
		case XHTTP:
			q := url.Values{"encryption": {"none"}, "security": {"reality"}, "sni": {p.SNI}, "fp": {"chrome"}, "pbk": {p.PublicKey}, "sid": {p.ShortID}, "type": {"xhttp"}, "path": {p.Path}, "mode": {"auto"}}
			link := fmt.Sprintf("vless://%s@%s?%s#%s", url.PathEscape(c.Secret), net.JoinHostPort(p.Address, strconv.Itoa(p.Port)), q.Encode(), url.QueryEscape(user.Name+" XHTTP"))
			out = append(out, Connection{Kind: p.Kind, Label: "VLESS + XHTTP + Reality", Format: "uri", Link: link})
		case Hysteria2:
			link := HysteriaLink(p, user.Name, fmt.Sprintf("u%d", user.ID), c.Secret)
			out = append(out, Connection{Kind: p.Kind, Label: "Hysteria2", Format: "uri", Link: link})
		case WireGuard, AmneziaWG:
			conf, err := ClientTunnel(p, c)
			if err != nil {
				return nil, err
			}
			label := "WireGuard"
			if p.Kind == AmneziaWG {
				label = "AmneziaWG 3.1"
			}
			out = append(out, Connection{Kind: p.Kind, Label: label, Format: "conf", Configuration: conf})
		}
	}
	return out, nil
}
func ClientTunnel(p store.Protocol, c store.ProtocolCredential) (string, error) {
	if err := Validate(p); err != nil {
		return "", err
	}
	if p.Kind != WireGuard && p.Kind != AmneziaWG {
		return "", ErrInvalid
	}
	if !validWGKey(c.PrivateKey) || !validWGKey(c.PublicKey) {
		return "", ErrInvalid
	}
	address, err := AddressForUser(p.Kind, c.UserID)
	if p.Kind == WireGuard {
		address = c.Address
		err = nil
		if !devices.WireGuardAddress(address) {
			return "", ErrInvalid
		}
	}
	if err != nil || c.Address != address {
		return "", ErrInvalid
	}
	extra := ""
	if p.Kind == AmneziaWG {
		extra = awgSettings(p.ObfsKey)
	}
	return fmt.Sprintf("[Interface]\nPrivateKey = %s\nAddress = %s/32\nDNS = 1.1.1.1\nMTU = 1280\n%s\n[Peer]\nPublicKey = %s\nEndpoint = %s\nAllowedIPs = 0.0.0.0/0\nPersistentKeepalive = 25\n", c.PrivateKey, address, extra, p.PublicKey, net.JoinHostPort(p.Address, strconv.Itoa(p.Port))), nil
}
func awgSettings(key string) string {
	return "Jc = 5\nJmin = 40\nJmax = 100\nS1 = 40\nS2 = 40\nS3 = 40\nS4 = 40\nH1 = 1\nH2 = 2\nH3 = 3\nH4 = 4\nHeaderProtectionKey = " + key + "\nContentPaddingAddition = 32-128\nRandomTrailers = off\nDisableCookies = off\n"
}
