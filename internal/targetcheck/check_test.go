package targetcheck

import (
	"context"
	"crypto/tls"
	"crypto/x509"
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
