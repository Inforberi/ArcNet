package noderuntime

import (
	"bytes"
	"crypto/ecdh"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"regexp"

	"github.com/arcvpn/arcvpn/internal/store"
	"github.com/arcvpn/arcvpn/internal/xray"
)

var cascadeClientID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// ValidateCascadeConfig is a separate operation boundary. Legacy direct-only
// requests never gain access to new forwarding fields. No paths/API/shell RPC.
func ValidateCascadeConfig(raw []byte) error {
	if len(raw) == 0 || len(raw) > MaxRequest-4096 {
		return errors.New("cascade config exceeds limit")
	}
	if err := uniqueJSON(raw); err != nil {
		return err
	}
	var root map[string]json.RawMessage
	if json.Unmarshal(raw, &root) != nil {
		return errors.New("invalid cascade JSON")
	}
	if err := allow(root, "log", "inbounds", "outbounds", "routing"); err != nil {
		return err
	}
	if err := xray.ValidateCascadeForwarding(raw); err != nil {
		return err
	}
	var inbounds []map[string]json.RawMessage
	if json.Unmarshal(root["inbounds"], &inbounds) != nil {
		return errors.New("invalid cascade listeners")
	}
	ids := map[string]bool{}
	privateKeys := map[string]bool{}
	ports := map[int]bool{}
	for idx, in := range inbounds {
		// Validate the established VLESS listener surface independently; replacing
		// the tag happens only in this validation copy, never in the applied bytes.
		in["tag"] = json.RawMessage(`"vless-reality"`)
		copyRoot := map[string]any{"log": root["log"], "inbounds": []any{in}, "outbounds": []any{map[string]string{"protocol": "freedom", "tag": "direct"}, map[string]string{"protocol": "blackhole", "tag": "block"}}}
		data, _ := json.Marshal(copyRoot)
		if err := ValidateConfig(data); err != nil {
			return err
		}
		var listener struct {
			Port     int `json:"port"`
			Settings struct {
				Clients []struct {
					ID string `json:"id"`
				} `json:"clients"`
			} `json:"settings"`
			Stream struct {
				Reality struct {
					Private string   `json:"privateKey"`
					Target  string   `json:"target"`
					Names   []string `json:"serverNames"`
					Short   []string `json:"shortIds"`
					Show    bool     `json:"show"`
				} `json:"realitySettings"`
			} `json:"streamSettings"`
		}
		item, _ := json.Marshal(in)
		if json.Unmarshal(item, &listener) != nil {
			return errors.New("invalid listener")
		}
		reality := listener.Stream.Reality
		keyBytes, err := base64.RawURLEncoding.DecodeString(reality.Private)
		if err != nil {
			return errors.New("invalid listener key")
		}
		key, err := ecdh.X25519().NewPrivateKey(keyBytes)
		if err != nil || len(reality.Names) != 1 || len(reality.Short) != 1 || reality.Show || privateKeys[reality.Private] || ports[listener.Port] {
			return errors.New("invalid or shared channel identity")
		}
		host, port, err := net.SplitHostPort(reality.Target)
		if err != nil || port != "443" {
			return errors.New("invalid Reality target")
		}
		profile := store.Reality{Address: "9.9.9.9", Port: listener.Port, Private: reality.Private, Public: base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes()), SNI: reality.Names[0], Target: host, ShortID: reality.Short[0]}
		if err := xray.Validate(profile); err != nil {
			return errors.New("invalid Reality listener identity")
		}
		privateKeys[reality.Private] = true
		ports[listener.Port] = true
		if idx == 1 && len(listener.Settings.Clients) != 1 {
			return errors.New("channel requires exactly one machine identity")
		}
		for _, client := range listener.Settings.Clients {
			if !cascadeClientID.MatchString(client.ID) || ids[client.ID] {
				return errors.New("invalid or reused VLESS identity")
			}
			ids[client.ID] = true
		}
	}
	// On an entry, machine credentials must also differ from every client.
	if len(inbounds) == 1 {
		var out struct {
			Outbounds []struct {
				Settings struct {
					ID string `json:"id"`
				} `json:"settings"`
			} `json:"outbounds"`
		}
		_ = json.Unmarshal(raw, &out)
		if ids[out.Outbounds[1].Settings.ID] {
			return errors.New("channel identity reused by a client")
		}
	}
	return nil
}

// Duplicate object keys would otherwise be silently normalized before policy
// checks. Reject them recursively, with a depth cap and one complete document.
func uniqueJSON(raw []byte) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	var walk func(int) error
	walk = func(depth int) error {
		if depth > 32 {
			return errors.New("JSON nesting limit")
		}
		token, err := d.Token()
		if err != nil {
			return err
		}
		delim, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		if delim != '{' && delim != '[' {
			return errors.New("unexpected JSON delimiter")
		}
		seen := map[string]bool{}
		for d.More() {
			if delim == '{' {
				key, err := d.Token()
				if err != nil {
					return err
				}
				name, ok := key.(string)
				if !ok || seen[name] {
					return errors.New("duplicate JSON key")
				}
				seen[name] = true
			}
			if err := walk(depth + 1); err != nil {
				return err
			}
		}
		_, err = d.Token()
		return err
	}
	if err := walk(0); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return errors.New("trailing JSON")
	}
	return nil
}
