package engines

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/arcvpn/arcvpn/internal/protocols"
	"github.com/arcvpn/arcvpn/internal/store"
)

func TestQuickConfigBlocksPrivilegeEscalation(t *testing.T) {
	for _, kind := range []string{protocols.WireGuard, protocols.AmneziaWG} {
		profile, err := protocols.New(context.Background(), kind, "198.51.100.41", 0, "", "", "")
		if err != nil {
			t.Fatal(err)
		}
		u := store.User{ID: 12, Enabled: true}
		cred, err := protocols.NewCredential(kind, u)
		if err != nil {
			t.Fatal(err)
		}
		data, err := protocols.Render(profile, []store.User{u}, []store.ProtocolCredential{cred})
		if err != nil {
			t.Fatal(err)
		}
		if kind == protocols.AmneziaWG {
			if !strings.Contains(string(data), "RandomTrailers = off") || !strings.Contains(string(data), "DisableCookies = off") {
				t.Fatal("missing safe AWG 3.1 defaults")
			}
		}
		if err := ValidateQuickServer(kind, data); err != nil {
			t.Fatalf("generated config rejected: %v", err)
		}
		for _, suffix := range []string{
			"\nPostUp = /bin/sh -c 'id > /tmp/root-owned'\n",
			"\nPreDown = /bin/sh\n",
			"\nDNS = 8.8.8.8\n",
			"\n[Peer]\nPublicKey = " + cred.PublicKey + "\nAllowedIPs = 0.0.0.0/0\n",
			"\n[Interface]\nPrivateKey = " + profile.PrivateKey + "\n",
		} {
			if err := ValidateQuickServer(kind, append(append([]byte(nil), data...), []byte(suffix)...)); err == nil {
				t.Fatalf("malicious quick directive accepted: %q", suffix)
			}
		}
		if kind == protocols.AmneziaWG {
			for _, mutation := range []struct{ old, new string }{
				{"RandomTrailers = off", "RandomTrailers = on"},
				{"DisableCookies = off", "DisableCookies = on"},
				{"S1 = 40", "S1 = 5"},
			} {
				tampered := strings.Replace(string(data), mutation.old, mutation.new, 1)
				if ValidateQuickServer(kind, []byte(tampered)) == nil {
					t.Fatalf("unsafe AWG3.1 mutation accepted: %s", mutation.new)
				}
			}
		}
		altered := strings.Replace(string(data), "ListenPort = ", "PostUp = /usr/bin/true\nListenPort = ", 1)
		if ValidateQuickServer(kind, []byte(altered)) == nil {
			t.Fatal("inline PostUp accepted")
		}
	}
}

func TestAWGPrivilegedCommandNeverInheritsEnvironment(t *testing.T) {
	cmd := awgCommand(context.Background(), awgGoBinary, awgInterface)
	if cmd.Path != awgGoBinary ||
		!reflect.DeepEqual(cmd.Args, []string{awgGoBinary, awgInterface}) {
		t.Fatalf("unexpected privileged command: %v", cmd.Args)
	}
	wantEnv := []string{
		"PATH=/opt/arcvpn/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"HOME=/root", "LANG=C",
	}
	if !reflect.DeepEqual(cmd.Env, wantEnv) {
		t.Fatalf("unsafe inherited env: %v", cmd.Env)
	}
}
