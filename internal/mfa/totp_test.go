package mfa

import (
	"strings"
	"testing"
	"time"
)

func TestRFC6238SHA1(t *testing.T) {
	secret := encoding.EncodeToString([]byte("12345678901234567890"))
	for _, tt := range []struct {
		sec      int64
		expected string
	}{{59, "94287082"}, {1111111109, "07081804"}, {1111111111, "14050471"}, {1234567890, "89005924"}, {2000000000, "69279037"}, {20000000000, "65353130"}} {
		value, err := Code(secret, tt.sec/30, 8)
		if err != nil || value != tt.expected {
			t.Fatalf("RFC vector %d: %s %v", tt.sec, value, err)
		}
	}
}
func TestTOTPReplayAndWindow(t *testing.T) {
	secret, err := NewSecret()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1800000060, 0)
	step := now.Unix() / 30
	code, _ := Code(secret, step, 6)
	if Verify(secret, code, now, -1) != step {
		t.Fatal("valid code rejected")
	}
	if Verify(secret, code, now, step) != -1 {
		t.Fatal("replayed code accepted")
	}
	if Verify(secret, "123", now, -1) != -1 {
		t.Fatal("bad code")
	}
	if !strings.Contains(ProvisioningURI(secret, "admin"), "otpauth://totp/") {
		t.Fatal("URI")
	}
}
