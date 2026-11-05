package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
)

// Passwords use PBKDF2-HMAC-SHA256 with per-user salt. A memory-hard KDF
// (Argon2id) is required before production exposure; see SECURITY.md.
const Iterations = 600000

func Secret(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
func UUID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
func HashToken(t string) string { s := sha256.Sum256([]byte(t)); return hex.EncodeToString(s[:]) }
func pbkdf2(password, salt []byte, iterations int, dkLen int) []byte {
	out := make([]byte, 0, dkLen)
	for block := uint32(1); len(out) < dkLen; block++ {
		mac := hmac.New(sha256.New, password)
		mac.Write(salt)
		mac.Write([]byte{byte(block >> 24), byte(block >> 16), byte(block >> 8), byte(block)})
		u := mac.Sum(nil)
		t := append([]byte(nil), u...)
		for j := 1; j < iterations; j++ {
			mac = hmac.New(sha256.New, password)
			mac.Write(u)
			u = mac.Sum(nil)
			for k := range t {
				t[k] ^= u[k]
			}
		}
		out = append(out, t...)
	}
	return out[:dkLen]
}
func HashPassword(password string) (string, error) {
	if len(password) < 16 || len(password) > 256 {
		return "", fmt.Errorf("password must be between 16 and 256 bytes")
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash := pbkdf2([]byte(password), salt, Iterations, 32)
	return fmt.Sprintf("pbkdf2-sha256$%d$%s$%s", Iterations, base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(hash)), nil
}
func VerifyPassword(password, stored string) bool {
	fields := strings.Split(stored, "$")
	if len(fields) != 4 || fields[0] != "pbkdf2-sha256" || fields[1] != "600000" || len(password) > 256 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(fields[2])
	if err != nil || len(salt) != 16 {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(fields[3])
	if err != nil || len(want) != 32 {
		return false
	}
	got := pbkdf2([]byte(password), salt, Iterations, 32)
	return subtle.ConstantTimeCompare(got, want) == 1
}
