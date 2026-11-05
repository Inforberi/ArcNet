package publicnet

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"testing"
)

type resolverFunc func(context.Context, string, string) ([]netip.Addr, error)

func (f resolverFunc) LookupNetIP(c context.Context, n, h string) ([]netip.Addr, error) {
	return f(c, n, h)
}

type dialFunc func(context.Context, string, string) (net.Conn, error)

func (f dialFunc) DialContext(c context.Context, n, a string) (net.Conn, error) { return f(c, n, a) }

func TestPublicIPAndURLBoundary(t *testing.T) {
	for _, raw := range []string{"example.com:443", "example.com/", "user@example.com", "example.com?x", "EXAMPLE.com", "example.com."} {
		if Hostname(raw) {
			t.Errorf("non-host accepted: %s", raw)
		}
	}
	for _, raw := range []string{"127.0.0.1", "10.1.1.1", "169.254.169.254", "100.100.100.200", "198.18.0.1", "192.0.2.1", "::1", "::ffff:127.0.0.1", "fc00::1", "fe80::1", "2001:db8::1", "2002:7f00:1::1", "64:ff9b::7f00:1"} {
		if PublicIP(netip.MustParseAddr(raw)) {
			t.Errorf("unsafe IP: %s", raw)
		}
	}
	for _, raw := range []string{"1.1.1.1", "8.8.8.8", "2606:4700:4700::1111"} {
		if !PublicIP(netip.MustParseAddr(raw)) {
			t.Errorf("public rejected: %s", raw)
		}
	}
	for _, raw := range []string{"http://example.com", "https://127.0.0.1", "https://example.com:8443", "https://a@b.com", "https://example.com/#fragment", "https://metadata.internal", "https://example.com/\n", "https://localhost.local"} {
		if ValidURL(raw) {
			t.Errorf("unsafe URL: %s", raw)
		}
	}
	if !ValidURL("https://example.com/catalog.json?version=2") {
		t.Fatal("public HTTPS URL rejected")
	}
}

func TestResolveRejectsMixedAnswersAndPinsCheckedIP(t *testing.T) {
	calls := 0
	dialed := ""
	g := Guard{Resolver: resolverFunc(func(context.Context, string, string) ([]netip.Addr, error) {
		calls++
		if calls == 1 {
			return []netip.Addr{netip.MustParseAddr("1.1.1.1")}, nil
		}
		return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
	}), Dialer: dialFunc(func(_ context.Context, _, a string) (net.Conn, error) { dialed = a; return nil, errors.New("fixture") })}
	_, _ = g.DialContext(context.Background(), "tcp", "example.com:443")
	if calls != 1 || dialed != "1.1.1.1:443" {
		t.Fatalf("DNS checked but hostname re-dialed: %d %q", calls, dialed)
	}
	g.Resolver = resolverFunc(func(context.Context, string, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("1.1.1.1"), netip.MustParseAddr("127.0.0.1")}, nil
	})
	dialed = ""
	if _, err := g.DialContext(context.Background(), "tcp", "example.com:443"); !errors.Is(err, ErrDenied) || dialed != "" {
		t.Fatal("mixed private/public answer dialed")
	}
	if _, err := g.Fetch(context.Background(), "https://127.0.0.1/", 1024); !errors.Is(err, ErrDenied) {
		t.Fatal("private fetch accepted")
	}
}
