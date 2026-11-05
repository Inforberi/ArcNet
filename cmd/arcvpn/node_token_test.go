package main

import (
	"strings"
	"testing"
)

func TestReadEnrollmentTokenInteractiveAndSSH(t *testing.T) {
	token := strings.Repeat("a", 43)
	for _, input := range []string{token + "\n", token + "\r\n", token} {
		got, err := readEnrollmentToken(strings.NewReader(input))
		if err != nil || got != token {
			t.Fatalf("token from stdin: %q %v", got, err)
		}
	}
	for _, input := range []string{"", "short\n", strings.Repeat("b", 130) + "\n", token + " shell\n"} {
		if _, err := readEnrollmentToken(strings.NewReader(input)); err == nil {
			t.Fatalf("accepted invalid input length %d", len(input))
		}
	}
}
