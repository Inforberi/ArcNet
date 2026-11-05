package nodeagent

import (
	"testing"
	"time"
)

func TestControlPlaneBackoffIsBoundedAndResets(t *testing.T) {
	if got := contactDelay(0); got != 20*time.Second {
		t.Fatalf("successful poll should reset to 20s, got %s", got)
	}
	cases := []struct {
		failures         int
		minimum, maximum time.Duration
	}{
		{1, 20 * time.Second, 24 * time.Second},
		{2, 40 * time.Second, 48 * time.Second},
		{3, 80 * time.Second, 96 * time.Second},
		{4, 160 * time.Second, 192 * time.Second},
		{5, 5 * time.Minute, 5 * time.Minute},
		{1000, 5 * time.Minute, 5 * time.Minute},
	}
	for _, tc := range cases {
		for trial := 0; trial < 50; trial++ {
			delay := contactDelay(tc.failures)
			if delay < tc.minimum || delay > tc.maximum {
				t.Fatalf("backoff after %d errors out of range: %v", tc.failures, delay)
			}
		}
	}
}
