package targetcheck

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"

	"github.com/arcvpn/arcvpn/internal/publicnet"
)

type testResolver struct{}

func (testResolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	return []netip.Addr{netip.MustParseAddr("1.1.1.1")}, nil
}

type testConnector struct{ address string }

func (d testConnector) DialContext(c context.Context, n, a string) (net.Conn, error) {
	return (&net.Dialer{}).DialContext(c, n, d.address)
}

type dualStackResolver struct{}

func (dualStackResolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	return []netip.Addr{netip.MustParseAddr("2606:4700:4700::1111"), netip.MustParseAddr("1.1.1.1")}, nil
}

type ipv4Connector struct{ address string }

func (d ipv4Connector) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, _, _ := net.SplitHostPort(address)
	if ip, e := netip.ParseAddr(host); e != nil || !ip.Is4() {
		return nil, errors.New("IPv6 route unavailable")
	}
	return (&net.Dialer{}).DialContext(ctx, network, d.address)
}
func TestCheckerUsesWorkingFamilyWithoutWeakeningTLS(t *testing.T) {
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	checker := Checker{Guard: publicnet.Guard{Resolver: dualStackResolver{}, Dialer: ipv4Connector{server.Listener.Addr().String()}}, roots: roots}
	report, err := checker.Check(context.Background(), []Candidate{{Host: "target.example.com", SNI: server.Certificate().DNSNames[0]}}, "local")
	if err != nil || report.Recommended == nil || report.Results[0].Successes != Samples || !report.Results[0].HTTPVerified {
		t.Fatal("working IPv4 path rejected", err, report)
	}
}

func TestCheckerTLSAndHTTPCompatibilityGates(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		h2, trust, wrongName bool
		maxTLS               uint16
		status               int
		eligible             bool
	}{
		{name: "valid", h2: true, trust: true, status: 200, eligible: true},
		{name: "untrusted", h2: true, status: 200},
		{name: "hostname mismatch", h2: true, trust: true, wrongName: true, status: 200},
		{name: "HTTP1 only", trust: true, status: 200},
		{name: "TLS12 only", h2: true, trust: true, maxTLS: tls.VersionTLS12, status: 200},
		{name: "redirect", h2: true, trust: true, status: 302},
		{name: "HTTP error", h2: true, trust: true, status: 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hits := 0
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hits++
				w.Header().Set("Location", "https://127.0.0.1/metadata")
				w.WriteHeader(tc.status)
			}))
			server.EnableHTTP2 = tc.h2
			server.Config.ErrorLog = log.New(io.Discard, "", 0)
			server.TLS = &tls.Config{MaxVersion: tc.maxTLS}
			server.StartTLS()
			defer server.Close()
			checker := Checker{Guard: publicnet.Guard{Resolver: testResolver{}, Dialer: testConnector{server.Listener.Addr().String()}}}
			if tc.trust {
				checker.roots = x509.NewCertPool()
				checker.roots.AddCert(server.Certificate())
			}
			sni := server.Certificate().DNSNames[0]
			if tc.wrongName {
				sni = "wrong.example.net"
			}
			report, err := checker.Check(context.Background(), []Candidate{{Host: "target.example.com", SNI: sni}}, "fixture-node")
			if err != nil || len(report.Results) != 1 {
				t.Fatalf("report: %#v %v", report, err)
			}
			result := report.Results[0]
			if result.Eligible != tc.eligible || (report.Recommended != nil) != tc.eligible || result.VPNVerified || report.VPNVerified {
				t.Fatalf("incorrect trust/proof: %#v", report)
			}
			if tc.eligible && (result.Successes != Samples || len(result.Samples) != Samples || !result.HTTPVerified || result.MedianMS <= 0 || result.P95MS < result.MedianMS || report.ExpiresAt-report.CheckedAt != ResultTTL) {
				t.Fatalf("invalid measurements: %#v", report)
			}
			if hits > 1 {
				t.Fatal("redirect followed or request retried unsafely")
			}
		})
	}
}

func TestCheckerCancelledBeforeDial(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	checker := Checker{Guard: publicnet.Guard{Resolver: testResolver{}, Dialer: testConnector{"127.0.0.1:1"}}}
	start := time.Now()
	report, err := checker.Check(ctx, []Candidate{{Host: "example.com"}}, "local")
	if err != nil || report.Recommended != nil || time.Since(start) > time.Second {
		t.Fatalf("cancellation: %#v %v", report, err)
	}
	if _, err := Normalize([]Candidate{{Host: "metadata.internal"}}); err == nil {
		t.Fatal("internal target accepted")
	}
}
