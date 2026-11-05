package xray

import (
	"encoding/json"
	"testing"

	"github.com/arcvpn/arcvpn/internal/store"
)

func TestXHTTPAddsIndependentInbound(t *testing.T) {
	reality := store.Reality{Address: "198.51.100.1", Port: 443, SNI: "www.cloudflare.com", Target: "www.cloudflare.com", Private: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", Public: "BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB", ShortID: "0123456789abcdef", Enabled: true}
	profile := store.Protocol{Kind: "vless-xhttp", Address: reality.Address, Port: 2087, SNI: reality.SNI, Target: reality.Target, PrivateKey: reality.Private, PublicKey: reality.Public, ShortID: reality.ShortID, Path: "/stream-test", Enabled: true}
	enabled := store.User{ID: 1, UUID: "f1aac612-1212-461a-ad27-a412d3a6c731", Enabled: true}
	disabled := store.User{ID: 2, UUID: "f1aac612-1212-461a-ad27-a412d3a6c732", Enabled: false}
	data, err := GenerateWithProfiles(reality, []store.User{enabled, disabled}, []store.Protocol{profile})
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Inbounds []struct {
			Protocol string `json:"protocol"`
			Port     int    `json:"port"`
			Settings struct {
				Clients []any `json:"clients"`
			} `json:"settings"`
			StreamSettings map[string]any `json:"streamSettings"`
		} `json:"inbounds"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	if len(config.Inbounds) != 2 {
		t.Fatalf("expected 2 inbounds: %d", len(config.Inbounds))
	}
	if config.Inbounds[1].Port != 2087 || len(config.Inbounds[1].Settings.Clients) != 1 || config.Inbounds[1].StreamSettings["network"] != "xhttp" {
		t.Fatal("XHTTP inbound differs from intended config")
	}
}
