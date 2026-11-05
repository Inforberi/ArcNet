// Package backup implements authenticated, versioned, password-encrypted
// portable ArcVPN state exports. It deliberately excludes administrators and
// web sessions; restoring them from a potentially stale backup would be unsafe.
package backup

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/netip"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/arcvpn/arcvpn/internal/protocols"
	"github.com/arcvpn/arcvpn/internal/routes"
	"github.com/arcvpn/arcvpn/internal/store"
	"github.com/arcvpn/arcvpn/internal/xray"
)

const (
	Version         = 4
	KDFIterations   = 210000
	MaxArchiveBytes = 10 << 20
	MaxUsers        = 10000
)

var uuidPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
var ErrInvalid = errors.New("invalid or incompatible ArcVPN backup")
var ErrPassword = errors.New("backup password is incorrect or archive was modified")

type VPN struct {
	Address    string `json:"address"`
	Port       int    `json:"port"`
	Target     string `json:"target"`
	SNI        string `json:"sni"`
	PrivateKey string `json:"private_key"`
	PublicKey  string `json:"public_key"`
	ShortID    string `json:"short_id"`
	Enabled    bool   `json:"enabled"`
}
type User struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	UUID      string `json:"uuid"`
	Token     string `json:"subscription_token"`
	Enabled   bool   `json:"enabled"`
	CreatedAt int64  `json:"created_at"`
}

// Protocol carries key material inside the authenticated encrypted payload.
// It is not the public API representation of a store.Protocol.
type Protocol struct {
	Kind       string `json:"kind"`
	Address    string `json:"address"`
	Port       int    `json:"port"`
	SNI        string `json:"sni"`
	Target     string `json:"target"`
	PrivateKey string `json:"private_key"`
	PublicKey  string `json:"public_key"`
	ShortID    string `json:"short_id"`
	Path       string `json:"path"`
	TLSCert    string `json:"tls_cert"`
	TLSKey     string `json:"tls_key"`
	ObfsKey    string `json:"obfs_key"`
	Enabled    bool   `json:"enabled"`
}
type Snapshot struct {
	Version     int                        `json:"version"`
	CreatedAt   int64                      `json:"created_at"`
	VPN         *VPN                       `json:"vpn"`
	Users       []User                     `json:"users"`
	Protocols   []Protocol                 `json:"protocols,omitempty"`
	Credentials []store.ProtocolCredential `json:"credentials,omitempty"`
	Nodes       []store.NodeIdentity       `json:"nodes,omitempty"`
	LogBudgetMB *int                       `json:"log_budget_mb,omitempty"`
	Routes      []routes.Policy            `json:"routes,omitempty"`
}
type Envelope struct {
	Format     string `json:"format"`
	Version    int    `json:"version"`
	KDF        string `json:"kdf"`
	Iterations int    `json:"iterations"`
	Salt       string `json:"salt"`
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}

func FromStore(v *store.Reality, users []store.User) Snapshot {
	result := Snapshot{Version: Version, CreatedAt: time.Now().Unix(), Users: make([]User, 0, len(users))}
	if v != nil {
		result.VPN = &VPN{Address: v.Address, Port: v.Port, Target: v.Target, SNI: v.SNI, PrivateKey: v.Private, PublicKey: v.Public, ShortID: v.ShortID, Enabled: v.Enabled}
	}
	for _, u := range users {
		result.Users = append(result.Users, User{ID: u.ID, Name: u.Name, UUID: u.UUID, Token: u.Subscription, Enabled: u.Enabled, CreatedAt: u.CreatedAt})
	}
	return result
}
func (s Snapshot) ToStore() (*store.Reality, []store.User) {
	var r *store.Reality
	if s.VPN != nil {
		v := s.VPN
		r = &store.Reality{Address: v.Address, Port: v.Port, Target: v.Target, SNI: v.SNI, Private: v.PrivateKey, Public: v.PublicKey, ShortID: v.ShortID, Enabled: v.Enabled}
	}
	users := make([]store.User, 0, len(s.Users))
	for _, u := range s.Users {
		users = append(users, store.User{ID: u.ID, Name: u.Name, UUID: u.UUID, Subscription: u.Token, Enabled: u.Enabled, CreatedAt: u.CreatedAt})
	}
	return r, users
}
func (s Snapshot) WithProtocols(profiles []store.Protocol, credentials []store.ProtocolCredential) Snapshot {
	s.Protocols = make([]Protocol, 0, len(profiles))
	for _, p := range profiles {
		s.Protocols = append(s.Protocols, Protocol{Kind: p.Kind, Address: p.Address, Port: p.Port, SNI: p.SNI, Target: p.Target, PrivateKey: p.PrivateKey, PublicKey: p.PublicKey, ShortID: p.ShortID, Path: p.Path, TLSCert: p.TLSCert, TLSKey: p.TLSKey, ObfsKey: p.ObfsKey, Enabled: p.Enabled})
	}
	s.Credentials = append([]store.ProtocolCredential(nil), credentials...)
	return s
}
func (s Snapshot) WithManagement(nodes []store.NodeIdentity, budget int) Snapshot {
	s.Nodes = append(make([]store.NodeIdentity, 0, len(nodes)), nodes...)
	s.LogBudgetMB = &budget
	return s
}
func (s Snapshot) ToProtocols() ([]store.Protocol, []store.ProtocolCredential) {
	profiles := make([]store.Protocol, 0, len(s.Protocols))
	for _, p := range s.Protocols {
		profiles = append(profiles, store.Protocol{Kind: p.Kind, Address: p.Address, Port: p.Port, SNI: p.SNI, Target: p.Target, PrivateKey: p.PrivateKey, PublicKey: p.PublicKey, ShortID: p.ShortID, Path: p.Path, TLSCert: p.TLSCert, TLSKey: p.TLSKey, ObfsKey: p.ObfsKey, Enabled: p.Enabled})
	}
	return profiles, append([]store.ProtocolCredential(nil), s.Credentials...)
}

func (s Snapshot) WithRoutes(policies []routes.Policy) Snapshot {
	s.Routes = make([]routes.Policy, len(policies))
	for i, p := range policies {
		p.BackupExits = append([]string{}, p.BackupExits...)
		s.Routes[i] = p
	}
	return s
}

func supportedVersion(v int) bool { return v >= 1 && v <= Version }

func (s Snapshot) Validate() error {
	if !supportedVersion(s.Version) || s.CreatedAt <= 0 || len(s.Users) > MaxUsers || (s.VPN == nil && (len(s.Users) > 0 || len(s.Protocols) > 0 || len(s.Credentials) > 0)) || len(s.Protocols) > len(protocols.Kinds()) || len(s.Credentials) > MaxUsers*len(protocols.Kinds()) || (s.Version == 1 && (len(s.Protocols) > 0 || len(s.Credentials) > 0)) || ((s.Version == 1 || s.Version == 2) && (len(s.Nodes) > 0 || s.LogBudgetMB != nil)) || len(s.Nodes) > 256 || (s.Version < 4 && len(s.Routes) != 0) {
		return ErrInvalid
	}
	if s.LogBudgetMB != nil && (*s.LogBudgetMB < 50 || *s.LogBudgetMB > 1024) {
		return ErrInvalid
	}
	nodeIDs, nodeTokens := map[string]bool{}, map[string]bool{}
	for _, node := range s.Nodes {
		if !uuidPattern.MatchString(node.ID) || len(node.Name) == 0 || len([]rune(node.Name)) > 64 || strings.ContainsAny(node.Name, "\x00\r\n") || node.CreatedAt <= 0 || len(node.TokenHash) != 64 || nodeIDs[node.ID] || nodeTokens[node.TokenHash] {
			return ErrInvalid
		}
		hash, err := hex.DecodeString(node.TokenHash)
		if err != nil || len(hash) != 32 {
			return ErrInvalid
		}
		nodeIDs[node.ID] = true
		nodeTokens[node.TokenHash] = true
	}
	if err := routes.ValidateGraph(s.Routes); err != nil {
		return fmt.Errorf("%w: routing graph: %v", ErrInvalid, err)
	}
	for _, p := range s.Routes {
		if p.Version < 1 || p.Version == math.MaxInt64 || p.UpdatedAt < 0 {
			return ErrInvalid
		}
		refs := append([]string{p.EntryNode}, p.BackupExits...)
		if p.PrimaryExit != "" {
			refs = append(refs, p.PrimaryExit)
		}
		for _, id := range refs {
			if id != routes.LocalNode && !nodeIDs[id] {
				return fmt.Errorf("%w: orphaned routing node", ErrInvalid)
			}
		}
	}
	if s.VPN != nil {
		r, _ := s.ToStore()
		if err := xray.Validate(*r); err != nil {
			return fmt.Errorf("%w: invalid VPN configuration: %v", ErrInvalid, err)
		}
	}
	ids, names, uuids, tokens := map[int64]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
	userUUID := make(map[int64]string, len(s.Users))
	for _, u := range s.Users {
		if u.ID <= 0 || !utf8.ValidString(u.Name) || strings.TrimSpace(u.Name) == "" || strings.ContainsAny(u.Name, "\r\n\x00") || len([]rune(u.Name)) < 1 || len([]rune(u.Name)) > 60 || !uuidPattern.MatchString(u.UUID) || len(u.Token) != 43 || u.CreatedAt <= 0 {
			return ErrInvalid
		}
		decoded, err := base64.RawURLEncoding.DecodeString(u.Token)
		if err != nil || len(decoded) != 32 {
			return ErrInvalid
		}
		if ids[u.ID] || names[u.Name] || uuids[u.UUID] || tokens[u.Token] {
			return fmt.Errorf("%w: duplicate users", ErrInvalid)
		}
		ids[u.ID] = true
		userUUID[u.ID] = u.UUID
		names[u.Name] = true
		uuids[u.UUID] = true
		tokens[u.Token] = true
	}
	// Every exported listener and credential is checked before the database is modified.
	// Reject unexpected, duplicate or orphaned entries, including duplicate endpoints.
	types := make(map[string]bool, len(s.Protocols))
	endpoints := map[string]bool{}
	if s.VPN != nil {
		endpoints[fmt.Sprintf("tcp/%s/%d", s.VPN.Address, s.VPN.Port)] = true
	}
	profiles, creds := s.ToProtocols()
	for _, p := range profiles {
		if types[p.Kind] || protocols.Validate(p) != nil {
			return fmt.Errorf("%w: invalid or duplicate protocol", ErrInvalid)
		}
		// The listener address is the local server's public IP, not an
		// arbitrary endpoint supplied by a backup file.
		if s.VPN == nil || p.Address != s.VPN.Address || (p.Kind == protocols.XHTTP && p.Port == 80) {
			return fmt.Errorf("%w: protocol endpoint incompatible with local server", ErrInvalid)
		}
		if (p.Kind == protocols.WireGuard || p.Kind == protocols.AmneziaWG) && !validTunnelKeypair(p.PrivateKey, p.PublicKey) {
			return ErrInvalid
		}
		types[p.Kind] = true
		endpoint := fmt.Sprintf("%s/%s/%d", protocols.Transport(p.Kind), p.Address, p.Port)
		if endpoints[endpoint] {
			return fmt.Errorf("%w: port conflict", ErrInvalid)
		}
		endpoints[endpoint] = true
	}
	pairs := map[string]bool{}
	tunnelAddresses := map[string]bool{}
	for _, c := range creds {
		key := fmt.Sprintf("%d/%s", c.UserID, c.Kind)
		if !ids[c.UserID] || !types[c.Kind] || pairs[key] {
			return fmt.Errorf("%w: invalid credential reference", ErrInvalid)
		}
		pairs[key] = true
		switch c.Kind {
		case protocols.XHTTP:
			// The Xray inbound authenticates the VPN user's UUID directly.
			// A different credential in the exported URI could never connect.
			if c.Secret != userUUID[c.UserID] {
				return ErrInvalid
			}
		case protocols.Hysteria2:
			if len(c.Secret) < 32 || len(c.Secret) > 1024 || strings.ContainsAny(c.Secret, "\r\n\t :\"'\\") {
				return ErrInvalid
			}
		case protocols.WireGuard, protocols.AmneziaWG:
			if !validTunnelKeypair(c.PrivateKey, c.PublicKey) {
				return ErrInvalid
			}
			address, e := protocols.AddressForUser(c.Kind, c.UserID)
			if e != nil || address != c.Address || tunnelAddresses[c.Kind+"/"+address] {
				return ErrInvalid
			}
			tunnelAddresses[c.Kind+"/"+address] = true
		default:
			return ErrInvalid
		}
	}
	for _, p := range profiles {
		for _, u := range s.Users {
			if !pairs[fmt.Sprintf("%d/%s", u.ID, p.Kind)] {
				return fmt.Errorf("%w: missing user credential", ErrInvalid)
			}
		}
	}
	return nil
}

func validTunnelKeypair(private, public string) bool {
	secret, e := base64.StdEncoding.DecodeString(private)
	if e != nil || len(secret) != 32 {
		return false
	}
	key, e := ecdh.X25519().NewPrivateKey(secret)
	if e != nil {
		return false
	}
	return base64.StdEncoding.EncodeToString(key.PublicKey().Bytes()) == public
}

// pbkdf2SHA256 is an intentionally small PBKDF2-HMAC-SHA256 implementation.
// An archive carries iteration count and is bounded before derivation.
func pbkdf2SHA256(password string, salt []byte, rounds int) []byte {
	msg := make([]byte, len(salt)+4)
	copy(msg, salt)
	msg[len(salt)+3] = 1
	mac := hmac.New(sha256.New, []byte(password))
	mac.Write(msg)
	u := mac.Sum(nil)
	key := append([]byte(nil), u...)
	for i := 1; i < rounds; i++ {
		mac.Reset()
		mac.Write(u)
		u = mac.Sum(u[:0])
		for j := range key {
			key[j] ^= u[j]
		}
	}
	return key
}
func Encrypt(s Snapshot, password string) ([]byte, error) {
	if len(password) < 12 || len(password) > 1024 {
		return nil, errors.New("backup password must contain 12–1024 bytes")
	}
	if err := s.Validate(); err != nil {
		return nil, err
	}
	body, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	if len(body) > MaxArchiveBytes/2 {
		return nil, ErrInvalid
	}
	salt := make([]byte, 16)
	nonce := make([]byte, 12)
	if _, err = rand.Read(salt); err != nil {
		return nil, err
	}
	if _, err = rand.Read(nonce); err != nil {
		return nil, err
	}
	key := pbkdf2SHA256(password, salt, KDFIterations)
	defer clear(key)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	envelope := Envelope{Format: "arcvpn-encrypted-state", Version: s.Version, KDF: "PBKDF2-HMAC-SHA256", Iterations: KDFIterations, Salt: base64.RawStdEncoding.EncodeToString(salt), Nonce: base64.RawStdEncoding.EncodeToString(nonce)}
	aad := []byte(fmt.Sprintf("%s:%d:%d", envelope.Format, envelope.Version, envelope.Iterations))
	envelope.Ciphertext = base64.RawStdEncoding.EncodeToString(aead.Seal(nil, nonce, body, aad))
	return json.MarshalIndent(envelope, "", "  ")
}
func Decrypt(blob []byte, password string) (Snapshot, error) {
	var result Snapshot
	if len(blob) > MaxArchiveBytes || len(password) < 12 || len(password) > 1024 {
		return result, ErrInvalid
	}
	var e Envelope
	if err := json.Unmarshal(blob, &e); err != nil {
		return result, ErrInvalid
	}
	if e.Format != "arcvpn-encrypted-state" || !supportedVersion(e.Version) || e.KDF != "PBKDF2-HMAC-SHA256" || e.Iterations != KDFIterations {
		return result, ErrInvalid
	}
	salt, es := base64.RawStdEncoding.DecodeString(e.Salt)
	nonce, en := base64.RawStdEncoding.DecodeString(e.Nonce)
	ct, ec := base64.RawStdEncoding.DecodeString(e.Ciphertext)
	if es != nil || en != nil || ec != nil || len(salt) != 16 || len(nonce) != 12 || len(ct) < 16 || len(ct) > MaxArchiveBytes/2 {
		return result, ErrInvalid
	}
	key := pbkdf2SHA256(password, salt, e.Iterations)
	defer clear(key)
	block, _ := aes.NewCipher(key)
	aead, _ := cipher.NewGCM(block)
	aad := []byte(fmt.Sprintf("%s:%d:%d", e.Format, e.Version, e.Iterations))
	plaintext, err := aead.Open(nil, nonce, ct, aad)
	if err != nil {
		return result, ErrPassword
	}
	defer clear(plaintext)
	if err = json.Unmarshal(plaintext, &result); err != nil {
		return Snapshot{}, ErrInvalid
	}
	if result.Version != e.Version {
		return Snapshot{}, ErrInvalid
	}
	if err = result.Validate(); err != nil {
		return Snapshot{}, err
	}
	return result, nil
}

// RemapAddress creates a copy of a portable backup targeting a different VPS.
// Secrets and subscription tokens stay stable; URLs have to be updated by the
// client using a reachable subscription endpoint (the old IP is not magic).
func (s Snapshot) RemapAddress(raw string) (Snapshot, error) {
	if raw == "" {
		return s, nil
	}
	ip, err := netip.ParseAddr(raw)
	if err != nil || !ip.Is4() || !ip.IsGlobalUnicast() || ip.IsPrivate() {
		return Snapshot{}, ErrInvalid
	}
	if s.VPN == nil {
		return Snapshot{}, ErrInvalid
	}
	s.VPN = &VPN{Address: s.VPN.Address, Port: s.VPN.Port, Target: s.VPN.Target, SNI: s.VPN.SNI, PrivateKey: s.VPN.PrivateKey, PublicKey: s.VPN.PublicKey, ShortID: s.VPN.ShortID, Enabled: s.VPN.Enabled}
	s.VPN.Address = raw
	s.Protocols = append([]Protocol(nil), s.Protocols...)
	for i := range s.Protocols {
		s.Protocols[i].Address = raw
	}
	if err := s.Validate(); err != nil {
		return Snapshot{}, err
	}
	return s, nil
}
