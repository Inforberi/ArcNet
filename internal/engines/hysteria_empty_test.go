package engines

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/arcvpn/arcvpn/internal/protocols"
	"github.com/arcvpn/arcvpn/internal/store"
)

func TestHysteriaEmptyRosterHasPrivateStableUnissuedCredential(t *testing.T) {
	m := Manager{Dir: t.TempDir()}
	for _, users := range [][]store.User{nil, {{ID: 1, Enabled: false}}} {
		data, err := protocols.Render(store.Protocol{Kind: protocols.Hysteria2, Address: "127.0.0.1", SNI: "127.0.0.1", Port: 8444, TLSCert: "/etc/arcvpn/tls/fullchain.pem", TLSKey: "/etc/arcvpn/tls/privkey.pem"}, users, nil)
		if err != nil {
			t.Fatal(err)
		}
		first, err := m.withHysteriaStatistics(data)
		if err != nil {
			t.Fatal(err)
		}
		second, err := m.withHysteriaStatistics(data)
		if err != nil || !bytes.Equal(first, second) {
			t.Fatal("empty roster configuration is unstable", err)
		}
		var cfg hysteriaConfig
		if err := json.Unmarshal(first, &cfg); err != nil {
			t.Fatal(err)
		}
		credential := cfg.Auth.Users["_arcnet_no_users"]
		if len(cfg.Auth.Users) != 1 || !statsSecretPattern.MatchString(credential) || credential == cfg.Traffic.Secret {
			t.Fatal("empty roster credential is missing or shares the Stats API key")
		}
		info, err := os.Stat(filepath.Join(m.Dir, "hysteria-stats.key"))
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatal("private seed permissions", err)
		}
	}
}
