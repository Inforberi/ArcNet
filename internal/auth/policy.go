package auth

import (
	"errors"
	"strings"
	"unicode/utf8"
)

const MinPasswordCharacters = 8
const MaxPasswordBytes = 256

// Count code points, not UTF-8 bytes or grapheme clusters. Never normalize or trim.
// Login deliberately does not call this new-password policy.
func ValidateNewPassword(password string) error {
	if !utf8.ValidString(password) || utf8.RuneCountInString(password) < MinPasswordCharacters || len(password) > MaxPasswordBytes || strings.ContainsAny(password, "\x00\r\n") {
		return errors.New("пароль: от 8 Unicode-символов, не более 256 байт UTF-8, без перевода строки")
	}
	return nil
}
