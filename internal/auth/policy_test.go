package auth

import (
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
)

func TestNewPasswordUnicodePolicy(t *testing.T) {
	for _, s := range []string{"12345678", "        ", "Пароль я", "😀😀😀😀😀😀😀😀", strings.Repeat("😀", 64), "e\u0301e\u0301e\u0301e\u0301"} {
		if ValidateNewPassword(s) != nil {
			t.Fatal("valid policy case rejected")
		}
	}
	for _, s := range []string{"1234567", "😀😀😀😀", strings.Repeat("😀", 65), strings.Repeat("x", 257), "12345678\n", "12345678\x00", string([]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff})} {
		if ValidateNewPassword(s) == nil {
			t.Fatal("invalid policy case accepted")
		}
	}
	password := " Пароль "
	hash, err := HashPassword(password)
	if err != nil || !VerifyPassword(password, hash) || VerifyPassword(strings.TrimSpace(password), hash) {
		t.Fatal("password was transformed")
	}
	// Imported historical hashes are verified without the new creation minimum.
	salt := []byte("1234567890123456")
	old := "old"
	value := pbkdf2([]byte(old), salt, Iterations, 32)
	encoded := fmt.Sprintf("pbkdf2-sha256$600000$%s$%s", base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(value))
	if !VerifyPassword(old, encoded) {
		t.Fatal("historical short password invalidated")
	}
}
