package routingprofile

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestImportRejectsUnsafeAndAmbiguousInput(t *testing.T) {
	p := Default()
	p.Name = "Мой профиль"
	p.ID = "external"
	p.Version = 99
	blob, _ := json.Marshal(Bundle{Format: "arcvpn-routing-profile", Version: 1, Profile: p})
	imported, err := Decode(blob)
	if err != nil || imported.ID != "" || imported.Version != 0 {
		t.Fatalf("import metadata: %#v %v", imported, err)
	}
	for _, bad := range [][]byte{append(append([]byte{}, blob...), []byte(" {}")...), []byte(strings.Replace(string(blob), `"version":1`, `"version":2`, 1)), []byte(strings.Replace(string(blob), `"format":`, `"unknown":true,"format":`, 1)), []byte(strings.Repeat(" ", MaxImportBytes+1))} {
		if _, err := Decode(bad); !errors.Is(err, ErrInvalid) {
			t.Fatalf("bad import accepted: %v", err)
		}
	}
	for _, bad := range []string{"https://example.com", "regexp:.*", "domain:bad..com", "geoip:ru\nfull:a.com", "192.0.2.1/99"} {
		if _, err := Token(bad); !errors.Is(err, ErrInvalid) {
			t.Fatalf("unsafe token accepted: %q", bad)
		}
	}
	p.Direct = []string{"Example.COM"}
	p.Block = []string{"domain:example.com"}
	if _, err := Normalize(p); !errors.Is(err, ErrInvalid) {
		t.Fatal("cross-action duplicate accepted")
	}
}

func TestExplainRespectsSwitchOrderAndUnknownGeodata(t *testing.T) {
	p := Default()
	p.Name = "Правила"
	p.DefaultAction = "direct"
	p.Block = []string{"domain:example.com"}
	d, err := Explain(p, "a.example.com")
	if err != nil || d.Action != "proxy" || d.NetworkVerified {
		t.Fatalf("off must retain VPN base: %#v %v", d, err)
	}
	p.Enabled = true
	d, err = Explain(p, "a.example.com")
	if err != nil || d.Action != "block" {
		t.Fatalf("subdomain match: %#v %v", d, err)
	}
	p.Block = []string{"geosite:ru"}
	p.Proxy = []string{"full:example.com"}
	d, err = Explain(p, "example.com")
	if err != nil || d.Action != "" || d.Status != "needs_dns_or_geodata" {
		t.Fatalf("unknown earlier rule silently skipped: %#v %v", d, err)
	}
	p.Order = []string{"proxy", "block", "direct"}
	d, err = Explain(p, "example.com")
	if err != nil || d.Action != "proxy" {
		t.Fatalf("order: %#v %v", d, err)
	}
	p.Block = []string{}
	p.Proxy = []string{"192.0.2.1/24"}
	p.DomainStrategy = "IPIfNonMatch"
	d, err = Explain(p, "other.example.com")
	if err != nil || d.Action != "" {
		t.Fatalf("DNS silently assumed: %#v %v", d, err)
	}
	d, err = Explain(p, "192.0.2.5")
	if err != nil || d.Action != "proxy" {
		t.Fatalf("CIDR: %#v %v", d, err)
	}
}

func TestListPreviewCanonicalizesAndBounds(t *testing.T) {
	entries, duplicates, err := ParseList("# comments\r\n EXAMPLE.com \r\ndomain:example.com\n192.0.2.42/24\n::ffff:192.0.2.5")
	if err != nil || duplicates != 1 || len(entries) != 3 || entries[1] != "192.0.2.0/24" || entries[2] != "192.0.2.5" {
		t.Fatalf("parse: %v %d %v", entries, duplicates, err)
	}
	if _, _, err := ParseList("example.com\nhttps://invalid.com"); err == nil || !strings.Contains(err.Error(), "2") {
		t.Fatal("missing line error")
	}
	if _, _, err := ParseList(strings.Repeat("a", MaxImportBytes+1)); err == nil {
		t.Fatal("oversize accepted")
	}
}
