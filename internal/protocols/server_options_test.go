package protocols

import (
	"context"
	"encoding/json"
	"testing"
)

func TestServerOptionsHysteriaRender(t *testing.T) {
	p, err := New(context.Background(), Hysteria2, "198.51.100.1", 8444, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	p.ServerOptions.UDPIdleSeconds = 90
	p.ServerOptions.QUICIdleSeconds = 45
	blob, err := Render(p, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	if err = json.Unmarshal(blob, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg["udpIdleTimeout"] != "90s" || cfg["quic"].(map[string]any)["maxIdleTimeout"] != "45s" {
		t.Fatal("timeouts lost")
	}
	if cfg["tls"].(map[string]any)["sniGuard"] != "dns-san" || cfg["auth"].(map[string]any)["type"] != "userpass" {
		t.Fatal("security defaults changed")
	}
	p.ServerOptions.QUICIdleSeconds = 121
	if _, err = Render(p, nil, nil); err == nil {
		t.Fatal("unsupported idle accepted")
	}
}
