package backup

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/arcvpn/arcvpn/internal/store"
)

func sample() Snapshot {
	token := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0x5e}, 32))
	return FromStore(&store.Reality{Address: "203.0.113.4", Port: 443, Target: "www.example.com", SNI: "www.example.com", Private: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", Public: "BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB", ShortID: "aabbccdd00112233", Enabled: true}, []store.User{{ID: 1, Name: "Тест", UUID: "12345678-1234-4234-8234-123456789abc", Subscription: token, CreatedAt: time.Now().Unix(), Enabled: true}})
}

func TestFingerprintBackupRoundTripAndLegacy(t *testing.T) {
	s := sample()
	s.VPN.Fingerprint = "firefox"
	s = s.WithProtocols([]store.Protocol{{Kind: "vless-xhttp", Address: s.VPN.Address, Port: 8444, SNI: s.VPN.SNI, Target: s.VPN.Target, PrivateKey: s.VPN.PrivateKey, PublicKey: s.VPN.PublicKey, ShortID: s.VPN.ShortID, Path: "/fixture", Fingerprint: "safari", Enabled: true}}, []store.ProtocolCredential{{UserID: s.Users[0].ID, Kind: "vless-xhttp", Secret: s.Users[0].UUID}})
	blob, err := Encrypt(s, "correct-horse-battery-something")
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decrypt(blob, "correct-horse-battery-something")
	if err != nil || got.VPN.Fingerprint != "firefox" {
		t.Fatal("fingerprint lost", err)
	}
	r, _ := got.ToStore()
	if r.Fingerprint != "firefox" {
		t.Fatal("restore lost preference")
	}
	profiles, _ := got.ToProtocols()
	if len(profiles) != 1 || profiles[0].Fingerprint != "safari" {
		t.Fatal("XHTTP backup lost preference")
	}
	remapped, err := got.RemapAddress("1.1.1.1")
	if err != nil || remapped.VPN.Fingerprint != "firefox" {
		t.Fatal("rebind lost preference", err)
	}
	s.Version = 12
	if s.Validate() == nil {
		t.Fatal("new field accepted under old backup version")
	}
	s.VPN.Fingerprint = ""
	s.Protocols[0].Fingerprint = ""
	if err := s.Validate(); err != nil {
		t.Fatal("legacy backup rejected", err)
	}
	s.Version = Version
	s.VPN.Fingerprint = "unsafe"
	if s.Validate() == nil {
		t.Fatal("unsafe fingerprint accepted")
	}
}
func TestEncryptDecryptAndValidation(t *testing.T) {
	orig := sample()
	password := "correct-horse-battery-something"
	blob, err := Encrypt(orig, password)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(blob, []byte("www.example.com")) || bytes.Contains(blob, []byte(orig.Users[0].Token)) {
		t.Fatal("sensitive values leaked to backup envelope")
	}
	out, err := Decrypt(blob, password)
	if err != nil {
		t.Fatal(err)
	}
	if out.Users[0].Token != orig.Users[0].Token || out.VPN.PrivateKey != orig.VPN.PrivateKey {
		t.Fatal("roundtrip lost credentials")
	}
	if _, err := Decrypt(blob, "wrong-long-passphrase"); !errors.Is(err, ErrPassword) {
		t.Fatalf("wrong password error: %v", err)
	}
	var env Envelope
	if err := json.Unmarshal(blob, &env); err != nil {
		t.Fatal(err)
	}
	env.Iterations = 2
	modified, _ := json.Marshal(env)
	if _, err = Decrypt(modified, password); !errors.Is(err, ErrInvalid) {
		t.Fatal("unsafe KDF params accepted")
	}
	orig.Users = append(orig.Users, orig.Users[0])
	if _, err = Encrypt(orig, password); !errors.Is(err, ErrInvalid) {
		t.Fatal("duplicate client accepted")
	}
}
func TestPBKDF2Vector(t *testing.T) {
	actual := pbkdf2SHA256("password", []byte("salt"), 2)
	expected, _ := hex.DecodeString("ae4d0c95af6b46d32d0adff928f06dd02a303f8ef3c251dfd6e2d85a95474c43")
	if !bytes.Equal(actual, expected) {
		t.Fatal("PBKDF2 implementation does not match known SHA256 test vector")
	}
	_ = sha256.Size
}
