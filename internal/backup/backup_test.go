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
