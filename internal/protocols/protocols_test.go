package protocols

import (
	"context"
	"crypto/ecdh"
	"encoding/base64"
	"net/url"
	"strings"
	"testing"

	"github.com/arcvpn/arcvpn/internal/store"
)

func TestWireGuardKeypairs(t *testing.T) {
	a, ap, err := WGKeys()
	if err != nil {
		t.Fatal(err)
	}
	b, bp, err := WGKeys()
	if err != nil {
		t.Fatal(err)
	}
	if a == b || ap == bp {
		t.Fatal("keypair reused")
	}
	for _, pair := range [][2]string{{a, ap}, {b, bp}} {
		secret, _ := base64.StdEncoding.DecodeString(pair[0])
		p, err := ecdh.X25519().NewPrivateKey(secret)
		if err != nil {
			t.Fatal(err)
		}
		public := base64.StdEncoding.EncodeToString(p.PublicKey().Bytes())
		if public != pair[1] {
			t.Fatal("public key does not match private key")
		}
	}
}

func TestWireGuardAndAmneziaConfig(t *testing.T) {
	for _, kind := range []string{WireGuard, AmneziaWG} {
		p, err := New(context.Background(), kind, "198.51.100.1", 0, "", "", "")
		if err != nil {
			t.Fatal(err)
		}
		if err := Validate(p); err != nil {
			t.Fatal(err)
		}
		user := store.User{ID: 10, UUID: "fe24b588-acf1-4a15-8c6b-41eaf439d622", Enabled: true}
		c, err := NewCredential(kind, user)
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasPrefix(c.Address, "10.") == false {
			t.Fatal("missing private address")
		}
		server, err := Render(p, []store.User{user}, []store.ProtocolCredential{c})
		if err != nil {
			t.Fatal(err)
		}
		client, err := ClientTunnel(p, c)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(server), "AllowedIPs = "+c.Address+"/32") {
			t.Fatal("server peer missing")
		}
		if !strings.Contains(client, "Endpoint = 198.51.100.1:") {
			t.Fatal("client endpoint missing")
		}
		if !strings.Contains(client, "AllowedIPs = 0.0.0.0/0") || strings.Contains(client, "::/0") {
			t.Fatal("client route must match IPv4-only server forwarding")
		}
		if kind == AmneziaWG && (!strings.Contains(client, "HeaderProtectionKey = ") || !strings.Contains(string(server), "ContentPaddingAddition = 32-128") || !strings.Contains(string(server), "RandomTrailers = off")) {
			t.Fatal("AWG3.1 config missing")
		}
		if strings.Contains(string(server), c.PrivateKey) {
			t.Fatal("client private key present on server")
		}
	}
}

func TestHysteriaSecretsAndURI(t *testing.T) {
	p, err := New(context.Background(), Hysteria2, "198.51.100.2", 0, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	u := store.User{ID: 2, Name: "Тест #1", Enabled: true}
	c, err := NewCredential(Hysteria2, u)
	if err != nil {
		t.Fatal(err)
	}
	conf, err := Render(p, []store.User{u}, []store.ProtocolCredential{c})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(conf), `"u2": "`+c.Secret+`"`) || !strings.Contains(string(conf), `"sniGuard": "dns-san"`) {
		t.Fatal("Hysteria userpass/TLS missing")
	}
	links, err := Connections(u, []store.Protocol{p}, []store.ProtocolCredential{c})
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 1 {
		t.Fatal("missing link")
	}
	parsed, err := url.Parse(links[0].Link)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Scheme != "hysteria2" || parsed.Host != "198.51.100.2:8444" || parsed.Query().Get("insecure") != "0" {
		t.Fatalf("insecure/malformed link: %s", parsed.Redacted())
	}
	if !strings.Contains(links[0].Link, "#%D0%A2") {
		t.Fatal("unicode label not encoded")
	}
}
func TestProtocolValidationRejectsMaliciousValues(t *testing.T) {
	p, err := New(context.Background(), Hysteria2, "198.51.100.2", 0, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	p.TLSKey = "/tmp/../../etc/shadow"
	if Validate(p) == nil {
		t.Fatal("untrusted cert path accepted")
	}
	if _, err := New(context.Background(), WireGuard, "; rm -rf /", 51820, "", "", ""); err == nil {
		t.Fatal("invalid IP accepted")
	}
	if _, err := AddressForUser(WireGuard, 65534); err == nil {
		t.Fatal("overflow allocation accepted")
	}
}
func TestClientDisabledAndMissingCredentials(t *testing.T) {
	p, _ := New(context.Background(), WireGuard, "198.51.100.1", 51820, "", "", "")
	u := store.User{ID: 1, Enabled: true}
	con, err := Connections(u, []store.Protocol{p}, nil)
	if err != nil || len(con) != 0 {
		t.Fatal("unprovisioned user leaked")
	}
	u.Enabled = false
	c, _ := NewCredential(WireGuard, u)
	con, err = Connections(u, []store.Protocol{p}, []store.ProtocolCredential{c})
	if err != nil || len(con) != 0 {
		t.Fatal("disabled user leaked")
	}
}
