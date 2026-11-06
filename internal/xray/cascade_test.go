package xray_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/arcvpn/arcvpn/internal/devices"
	"github.com/arcvpn/arcvpn/internal/noderuntime"
	"github.com/arcvpn/arcvpn/internal/store"
	"github.com/arcvpn/arcvpn/internal/xray"
)

func cascadePair(t *testing.T) ([]byte, []byte, store.Reality, store.Reality, xray.CascadeChannel) {
	t.Helper()
	first, e := xray.NewCascadeChannel(11443)
	if e != nil {
		t.Fatal(e)
	}
	second, e := xray.NewCascadeChannel(12443)
	if e != nil {
		t.Fatal(e)
	}
	hop, e := xray.NewCascadeChannel(13443)
	if e != nil {
		t.Fatal(e)
	}
	a := store.Reality{Address: "9.9.9.9", Port: 11443, Target: "www.example.com", SNI: "www.example.com", Private: first.Private, Public: first.Public, ShortID: first.ShortID, Enabled: true}
	b := store.Reality{Address: "1.1.1.1", Port: 12443, Target: "www.example.com", SNI: "www.example.com", Private: second.Private, Public: second.Public, ShortID: second.ShortID, Enabled: true}
	users := []store.User{{ID: 1, UUID: "11111111-1111-4111-8111-111111111111", Enabled: true}, {ID: 2, UUID: "22222222-2222-4222-8222-222222222222", Enabled: false}}
	left, right, e := xray.GenerateCascade(a, b, users, users, hop)
	if e != nil {
		t.Fatal(e)
	}
	return left, right, a, b, hop
}
func TestCascadePairIndependentAndFailClosed(t *testing.T) {
	left, right, _, _, hop := cascadePair(t)
	for _, raw := range [][]byte{left, right} {
		if e := noderuntime.ValidateCascadeConfig(raw); e != nil {
			t.Fatal(e)
		}
	}
	if strings.Contains(string(left), `"freedom"`) || strings.Contains(string(left), hop.Private) {
		t.Fatal("entry exposes direct fallback or exit private key")
	}
	if strings.Contains(string(right), "22222222-2222-4222-8222-222222222222") {
		t.Fatal("disabled user restored")
	}
	if !strings.Contains(string(right), "11111111-1111-4111-8111-111111111111") || !strings.Contains(string(right), hop.ID) {
		t.Fatal("existing or channel credentials lost")
	}
	if e := noderuntime.ValidateConfig(left); e == nil {
		t.Fatal("legacy operation accepted cascade")
	}
	if raw, _ := json.Marshal(hop); string(raw) != "{}" {
		t.Fatal("machine identity serialized into public DTO")
	}
}
func TestCascadeRejectsIdentityReuse(t *testing.T) {
	_, _, a, b, hop := cascadePair(t)
	for _, users := range [][]store.User{
		{{UUID: hop.ID, Enabled: true}},
		{{UUID: "11111111-1111-4111-8111-111111111111", Enabled: true, DeviceAccess: &devices.Record{Devices: []devices.Device{{UUID: hop.ID}}}}},
	} {
		if _, _, e := xray.GenerateCascade(a, b, users, nil, hop); e == nil {
			t.Fatal("reused identity accepted")
		}
	}
	for _, mutate := range []func(*store.Reality, *xray.CascadeChannel){
		func(b *store.Reality, c *xray.CascadeChannel) { b.Address = a.Address },
		func(b *store.Reality, c *xray.CascadeChannel) { b.Address = "127.0.0.1" },
		func(b *store.Reality, c *xray.CascadeChannel) { c.Port = b.Port },
		func(b *store.Reality, c *xray.CascadeChannel) { c.Public = a.Public },
	} {
		other, channel := b, hop
		mutate(&other, &channel)
		if _, _, e := xray.GenerateCascade(a, other, nil, nil, channel); e == nil {
			t.Fatal("invalid channel accepted")
		}
	}
}
func TestCascadeRejectsBypassesAndPrivilegedFields(t *testing.T) {
	left, right, _, _, _ := cascadePair(t)
	for name, raw := range map[string][]byte{"entry": left, "exit": right} {
		t.Run(name, func(t *testing.T) {
			mutations := []func(map[string]any){
				func(v map[string]any) { v["api"] = map[string]any{"tag": "admin"} },
				func(v map[string]any) { v["routing"].(map[string]any)["rules"] = []any{} },
				func(v map[string]any) {
					v["outbounds"].([]any)[0] = map[string]any{"protocol": "freedom", "tag": "direct"}
				},
				func(v map[string]any) {
					v["outbounds"].([]any)[1].(map[string]any)["proxySettings"] = map[string]any{"tag": "direct"}
				},
				func(v map[string]any) {
					v["inbounds"].([]any)[0].(map[string]any)["streamSettings"].(map[string]any)["sockopt"] = map[string]any{"mark": 1}
				},
			}
			for _, mutate := range mutations {
				var v map[string]any
				_ = json.Unmarshal(raw, &v)
				mutate(v)
				data, _ := json.Marshal(v)
				if e := noderuntime.ValidateCascadeConfig(data); e == nil {
					t.Fatal("unsafe config accepted")
				}
			}
			duplicate := append([]byte(`{"routing":{},`), raw[1:]...)
			if e := noderuntime.ValidateCascadeConfig(duplicate); e == nil {
				t.Fatal("duplicate JSON key accepted")
			}
		})
	}
}
