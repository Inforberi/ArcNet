// Package publicnet enforces the shared SSRF boundary for user-requested public
// HTTPS downloads and target diagnostics. It never uses environment proxies.
package publicnet

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/arcvpn/arcvpn/internal/nodeprotocol"
)

var ErrDenied = errors.New("цель не является разрешённым публичным HTTPS-адресом")

type Resolver interface {
	LookupNetIP(context.Context, string, string) ([]netip.Addr, error)
}
type Connector interface {
	DialContext(context.Context, string, string) (net.Conn, error)
}
type Guard struct {
	Resolver Resolver
	Dialer   Connector
}

func New() Guard {
	return Guard{Resolver: net.DefaultResolver, Dialer: &net.Dialer{Timeout: 3 * time.Second}}
}
func PublicIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	for _, raw := range []string{"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8", "169.254.0.0/16", "172.16.0.0/12", "192.0.0.0/24", "192.0.2.0/24", "192.88.99.0/24", "192.168.0.0/16", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "224.0.0.0/4", "240.0.0.0/4", "2001::/23", "2001:db8::/32", "2002::/16", "3fff::/20"} {
		if netip.MustParsePrefix(raw).Contains(ip) {
			return false
		}
	}
	return ip.Is4() || netip.MustParsePrefix("2000::/3").Contains(ip)
}
func Hostname(host string) bool {
	if len(host) > 253 || !strings.Contains(host, ".") || strings.ContainsAny(host, ":/@?#[]") || host != strings.ToLower(host) || strings.HasSuffix(host, ".") {
		return false
	}
	if _, err := netip.ParseAddr(host); err == nil {
		return false
	}
	_, err := nodeprotocol.ValidateHTTPSOrigin("https://" + host)
	return err == nil && !strings.HasSuffix(host, ".localhost") && !strings.HasSuffix(host, ".local") && !strings.HasSuffix(host, ".internal")
}
func (g Guard) Resolve(ctx context.Context, host string) ([]netip.Addr, error) {
	if !Hostname(host) {
		return nil, ErrDenied
	}
	addresses, err := g.Resolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil, errors.New("не удалось разрешить DNS цели")
	}
	if len(addresses) == 0 || len(addresses) > 32 {
		return nil, ErrDenied
	}
	// Reject mixed public/private answers, not just the address selected for dial.
	for _, ip := range addresses {
		if !PublicIP(ip) {
			return nil, ErrDenied
		}
	}
	return addresses, nil
}
func (g Guard) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil || port != "443" {
		return nil, ErrDenied
	}
	addresses, err := g.Resolve(ctx, host)
	if err != nil {
		return nil, err
	}
	var last error
	for i, ip := range addresses {
		if i >= 2 {
			break
		}
		conn, err := g.Dialer.DialContext(ctx, "tcp", net.JoinHostPort(ip.String(), "443"))
		if err == nil {
			return conn, nil
		}
		last = err
	}
	return nil, last
}
func ValidURL(raw string) bool {
	if len(raw) > 2048 || strings.ContainsAny(raw, "\x00\r\n\\") {
		return false
	}
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.User == nil && u.Fragment == "" && u.Opaque == "" && u.Port() == "" && Hostname(u.Hostname())
}
func (g Guard) Fetch(ctx context.Context, raw string, limit int64) ([]byte, error) {
	if !ValidURL(raw) || limit < 1 || limit > 4<<20 {
		return nil, ErrDenied
	}
	transport := &http.Transport{Proxy: nil, DialContext: g.DialContext, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}, TLSHandshakeTimeout: 4 * time.Second, ResponseHeaderTimeout: 5 * time.Second, DisableCompression: true, DisableKeepAlives: true, MaxResponseHeaderBytes: 16 << 10}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, ErrDenied
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, errors.New("не удалось безопасно загрузить HTTPS-источник")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return nil, errors.New("источник должен вернуть HTTP 200 без перенаправлений")
	}
	if response.ContentLength > limit {
		return nil, errors.New("источник превышает лимит размера")
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil || int64(len(body)) > limit {
		return nil, errors.New("источник повреждён или превышает лимит размера")
	}
	return body, nil
}
