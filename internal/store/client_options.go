package store

import (
	"encoding/json"
	"errors"
	"net/netip"
	"strings"

	"github.com/arcvpn/arcvpn/internal/db"
)

// ClientOptions affects exports only, never root helpers or server interfaces.
type ClientOptions struct {
	XHTTPMode string `json:"xhttp_mode,omitempty"`
	MTU       int    `json:"mtu,omitempty"`
	DNS       string `json:"dns,omitempty"`
	Keepalive *int   `json:"keepalive,omitempty"`
}

func ValidateClientOptions(kind string, o ClientOptions) error {
	kind = ConnectionKind(kind)
	invalid := errors.New("invalid client export options")
	switch kind {
	case "vless-xhttp":
		if o.MTU != 0 || o.DNS != "" || o.Keepalive != nil {
			return invalid
		}
		switch o.XHTTPMode {
		case "", "auto", "packet-up", "stream-up", "stream-one":
			return nil
		}
		return invalid
	case "wireguard":
		if o.XHTTPMode != "" || (o.MTU != 0 && (o.MTU < 1280 || o.MTU > 1500)) || (o.Keepalive != nil && (*o.Keepalive < 0 || *o.Keepalive > 65535)) {
			return invalid
		}
		if o.DNS == "" {
			return nil
		}
		if len(o.DNS) > 200 || strings.ContainsAny(o.DNS, "\r\n") {
			return invalid
		}
		values := strings.Split(o.DNS, ",")
		if len(values) > 4 {
			return invalid
		}
		for _, value := range values {
			ip, err := netip.ParseAddr(strings.TrimSpace(value))
			if err != nil || !ip.Is4() || ip.IsUnspecified() || ip.IsMulticast() {
				return invalid
			}
		}
		return nil
	default:
		if o != (ClientOptions{}) {
			return invalid
		}
		return nil
	}
}

func (o ClientOptions) Mode() string {
	if o.XHTTPMode == "" {
		return "auto"
	}
	return o.XHTTPMode
}
func (o ClientOptions) TunnelMTU() int {
	if o.MTU == 0 {
		return 1280
	}
	return o.MTU
}
func (o ClientOptions) TunnelDNS() string {
	if o.DNS == "" {
		return "1.1.1.1"
	}
	return o.DNS
}
func (o ClientOptions) TunnelKeepalive() int {
	if o.Keepalive == nil {
		return 25
	}
	return *o.Keepalive
}

func readClientOptions(tx *db.Tx, kind string) (ClientOptions, error) {
	var o ClientOptions
	rows, err := tx.Query("SELECT value FROM settings WHERE key=?", "client_options:"+kind)
	if err != nil || len(rows) == 0 {
		return o, err
	}
	if err = json.Unmarshal([]byte(rows[0]["value"]), &o); err != nil {
		return o, errors.New("invalid stored client options")
	}
	return o, ValidateClientOptions(kind, o)
}
func saveClientOptions(tx *db.Tx, kind string, o ClientOptions) error {
	if err := ValidateClientOptions(kind, o); err != nil {
		return err
	}
	if o == (ClientOptions{}) {
		return tx.Exec("DELETE FROM settings WHERE key=?", "client_options:"+kind)
	}
	value, err := json.Marshal(o)
	if err != nil {
		return err
	}
	return tx.Exec("INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", "client_options:"+kind, string(value))
}
func clearProtocolClientOptions(tx *db.Tx) error {
	return tx.Exec("DELETE FROM settings WHERE key LIKE 'client_options:%' AND key != 'client_options:vless-reality'")
}
