package store

import (
	"encoding/json"
	"errors"

	"github.com/arcvpn/arcvpn/internal/db"
)

type ServerOptions struct {
	Flow            string `json:"flow,omitempty"`
	Salamander      bool   `json:"salamander,omitempty"`
	UpMbps          int    `json:"up_mbps,omitempty"`
	DownMbps        int    `json:"down_mbps,omitempty"`
	Sniffing        *bool  `json:"sniffing,omitempty"`
	UDPIdleSeconds  int    `json:"udp_idle_seconds,omitempty"`
	QUICIdleSeconds int    `json:"quic_idle_seconds,omitempty"`
}

func (o ServerOptions) Equal(other ServerOptions) bool {
	return o.Flow == other.Flow && o.Salamander == other.Salamander && o.UpMbps == other.UpMbps && o.DownMbps == other.DownMbps && o.UDPIdleSeconds == other.UDPIdleSeconds && o.QUICIdleSeconds == other.QUICIdleSeconds && ((o.Sniffing == nil && other.Sniffing == nil) || (o.Sniffing != nil && other.Sniffing != nil && *o.Sniffing == *other.Sniffing))
}

func ValidateServerOptions(kind string, o ServerOptions) error {
	valid := false
	switch kind {
	case "vless-reality", "vless-xhttp":
		valid = o.UDPIdleSeconds == 0 && o.QUICIdleSeconds == 0 && !o.Salamander && o.UpMbps == 0 && o.DownMbps == 0 && (o.Flow == "" || (kind == "vless-reality" && (o.Flow == "none" || o.Flow == "xtls-rprx-vision")))
	case "hysteria2":
		valid = o.Flow == "" && o.UpMbps >= 0 && o.UpMbps <= 100000 && o.DownMbps >= 0 && o.DownMbps <= 100000 && o.Sniffing == nil && (o.UDPIdleSeconds == 0 || (o.UDPIdleSeconds >= 2 && o.UDPIdleSeconds <= 600)) && (o.QUICIdleSeconds == 0 || (o.QUICIdleSeconds >= 4 && o.QUICIdleSeconds <= 120))
	default:
		valid = o == (ServerOptions{})
	}
	if !valid {
		return errors.New("invalid server options")
	}
	return nil
}
func (o ServerOptions) VLESSFlow() string {
	if o.Flow == "none" {
		return ""
	}
	return "xtls-rprx-vision"
}
func (o ServerOptions) SniffingEnabled(kind string) bool {
	if o.Sniffing != nil {
		return *o.Sniffing
	}
	return kind == "vless-reality"
}
func readServerOptions(tx *db.Tx, kind string) (ServerOptions, error) {
	var o ServerOptions
	rows, err := tx.Query("SELECT value FROM settings WHERE key=?", "server_options:"+kind)
	if err != nil || len(rows) == 0 {
		return o, err
	}
	if json.Unmarshal([]byte(rows[0]["value"]), &o) != nil {
		return o, errors.New("invalid stored server options")
	}
	return o, ValidateServerOptions(kind, o)
}
func saveServerOptions(tx *db.Tx, kind string, o ServerOptions) error {
	if err := ValidateServerOptions(kind, o); err != nil {
		return err
	}
	if o == (ServerOptions{}) {
		return tx.Exec("DELETE FROM settings WHERE key=?", "server_options:"+kind)
	}
	value, err := json.Marshal(o)
	if err != nil {
		return err
	}
	return tx.Exec("INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", "server_options:"+kind, string(value))
}
func clearProtocolServerOptions(tx *db.Tx) error {
	return tx.Exec("DELETE FROM settings WHERE key IN ('server_options:vless-xhttp','server_options:hysteria2','server_options:wireguard','server_options:amneziawg')")
}
