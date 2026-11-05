// Package mfa implements RFC 6238 compatible 6-digit TOTP for administrator login.
package mfa

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

var encoding = base32.StdEncoding.WithPadding(base32.NoPadding)

const period int64 = 30

func NewSecret() (string, error) {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return encoding.EncodeToString(b), nil
}

func ProvisioningURI(secret, username string) string {
	label := "ArcVPN:" + username
	v := url.Values{"secret": {secret}, "issuer": {"ArcVPN"}, "algorithm": {"SHA1"}, "digits": {"6"}, "period": {"30"}}
	return "otpauth://totp/" + url.PathEscape(label) + "?" + v.Encode()
}

func Code(secret string, step int64, digits int) (string, error) {
	if step < 0 || (digits != 6 && digits != 8) {
		return "", errors.New("invalid TOTP parameters")
	}
	key, err := encoding.DecodeString(strings.ToUpper(secret))
	if err != nil || len(key) < 16 {
		return "", errors.New("invalid TOTP secret")
	}
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(step))
	h := hmac.New(sha1.New, key)
	_, _ = h.Write(msg[:])
	sum := h.Sum(nil)
	offset := int(sum[len(sum)-1] & 15)
	truncated := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	modulus := uint32(1000000)
	if digits == 8 {
		modulus = 100000000
	}
	return fmt.Sprintf("%0*d", digits, truncated%modulus), nil
}

// Verify returns a usable time step, or -1. Steps at/before lastAccepted
// are rejected to prevent replay; comparison is constant-time.
func Verify(secret, input string, now time.Time, lastAccepted int64) int64 {
	if len(input) != 6 {
		return -1
	}
	for i := range input {
		if input[i] < '0' || input[i] > '9' {
			return -1
		}
	}
	current := now.Unix() / period
	for _, step := range []int64{current - 1, current, current + 1} {
		if step <= lastAccepted || step < 0 {
			continue
		}
		expected, err := Code(secret, step, 6)
		if err == nil && subtle.ConstantTimeCompare([]byte(expected), []byte(input)) == 1 {
			return step
		}
	}
	return -1
}
