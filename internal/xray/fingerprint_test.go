package xray

import (
	"net/url"
	"testing"

	"github.com/arcvpn/arcvpn/internal/store"
)

func TestClientFingerprintContract(t *testing.T) {
	for _, value := range []string{"", "chrome", "firefox", "safari", "ios", "android", "edge", "360", "qq", "random", "randomized"} {
		if !ValidFingerprint(value) {
			t.Fatal("supported fingerprint rejected", value)
		}
		raw, err := url.Parse(Link(store.Reality{Address: "198.51.100.1", Port: 443, Fingerprint: value}, store.User{UUID: "fixture"}))
		if err != nil || raw.Query().Get("fp") != ClientFingerprint(value) {
			t.Fatal("fingerprint lost from export", value, err)
		}
	}
	for _, value := range []string{"unsafe", "Firefox", " firefox", "firefox&security=none", "chrome\n"} {
		if ValidFingerprint(value) {
			t.Fatal("invalid fingerprint accepted", value)
		}
	}
}
