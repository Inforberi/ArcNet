// Package targetcheck performs read-only REALITY target compatibility checks.
// It never changes Xray and never claims that a VPN client handshake succeeded.
package targetcheck

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"math"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/arcvpn/arcvpn/internal/publicnet"
)

const Samples = 3
const MaxCandidates = 4
const ResultTTL = 600

type Candidate struct {
	Host string `json:"host"`
	SNI  string `json:"sni"`
}
type Measurement struct {
	TCPMS float64 `json:"tcp_ms"`
	TLSMS float64 `json:"tls_ms"`
}
type Result struct {
	Candidate    Candidate     `json:"candidate"`
	Eligible     bool          `json:"eligible"`
	Successes    int           `json:"successes"`
	Samples      []Measurement `json:"samples"`
	MedianMS     float64       `json:"median_ms"`
	P95MS        float64       `json:"p95_ms"`
	Error        string        `json:"error,omitempty"`
	HTTPStatus   int           `json:"http_status"`
	HTTPVerified bool          `json:"http_verified"`
	CheckedAt    int64         `json:"checked_at"`
	VPNVerified  bool          `json:"vpn_verified"`
}
type Report struct {
	Results      []Result   `json:"results"`
	Recommended  *Candidate `json:"recommended"`
	MeasuredFrom string     `json:"measured_from"`
	CheckedAt    int64      `json:"checked_at"`
	ExpiresAt    int64      `json:"expires_at"`
	VPNVerified  bool       `json:"vpn_verified"`
}
type Checker struct {
	Guard publicnet.Guard
	roots *x509.CertPool
}

func Normalize(items []Candidate) ([]Candidate, error) {
	if len(items) < 1 || len(items) > MaxCandidates {
		return nil, errors.New("за одну проверку выберите от 1 до 4 целей")
	}
	result := []Candidate{}
	seen := map[string]bool{}
	for _, item := range items {
		item.Host = strings.ToLower(strings.TrimSpace(item.Host))
		item.SNI = strings.ToLower(strings.TrimSpace(item.SNI))
		if item.SNI == "" {
			item.SNI = item.Host
		}
		if !publicnet.Hostname(item.Host) || !publicnet.Hostname(item.SNI) {
			return nil, errors.New("target и SNI должны быть корректными публичными DNS-именами")
		}
		key := item.Host + "/" + item.SNI
		if seen[key] {
			return nil, errors.New("цели не должны повторяться")
		}
		seen[key] = true
		result = append(result, item)
	}
	return result, nil
}
func (checker Checker) Check(ctx context.Context, candidates []Candidate, from string) (Report, error) {
	candidates, err := Normalize(candidates)
	if err != nil {
		return Report{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 24*time.Second)
	defer cancel()
	report := Report{Results: make([]Result, len(candidates)), MeasuredFrom: from, CheckedAt: time.Now().Unix()}
	report.ExpiresAt = report.CheckedAt + ResultTTL
	var wg sync.WaitGroup
	slots := make(chan struct{}, 2)
	for index, item := range candidates {
		wg.Add(1)
		go func(i int, c Candidate) {
			defer wg.Done()
			select {
			case slots <- struct{}{}:
				defer func() { <-slots }()
			case <-ctx.Done():
				report.Results[i] = Result{Candidate: c, Samples: []Measurement{}, CheckedAt: report.CheckedAt, Error: "Время проверки истекло"}
				return
			}
			report.Results[i] = checker.checkOne(ctx, c)
		}(index, item)
	}
	wg.Wait()
	for _, result := range report.Results {
		if result.Eligible && (report.Recommended == nil || result.MedianMS < medianFor(report.Results, *report.Recommended)) {
			candidate := result.Candidate
			report.Recommended = &candidate
		}
	}
	return report, nil
}
func medianFor(results []Result, c Candidate) float64 {
	for _, result := range results {
		if result.Candidate == c {
			return result.MedianMS
		}
	}
	return math.Inf(1)
}
func (checker Checker) checkOne(ctx context.Context, c Candidate) Result {
	result := Result{Candidate: c, Samples: []Measurement{}, CheckedAt: time.Now().Unix()}
	resolveCtx, stop := context.WithTimeout(ctx, 3*time.Second)
	_, err := checker.Guard.Resolve(resolveCtx, c.Host)
	stop()
	if err != nil {
		result.Error = err.Error()
		return result
	}
	for i := 0; i < Samples; i++ {
		sampleCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		start := time.Now()
		connection, err := checker.Guard.DialContext(sampleCtx, "tcp", net.JoinHostPort(c.Host, "443"))
		if err != nil {
			cancel()
			result.Error = "TCP-соединение не установлено"
			continue
		}
		if deadline, ok := sampleCtx.Deadline(); ok {
			_ = connection.SetDeadline(deadline)
		}
		tcpDuration := time.Since(start)
		secure := tls.Client(connection, &tls.Config{ServerName: c.SNI, MinVersion: tls.VersionTLS13, RootCAs: checker.roots, NextProtos: []string{"h2"}})
		start = time.Now()
		err = secure.HandshakeContext(sampleCtx)
		tlsDuration := time.Since(start)
		if err == nil {
			state := secure.ConnectionState()
			if state.Version != tls.VersionTLS13 || state.NegotiatedProtocol != "h2" {
				err = errors.New("target не подтвердил TLS 1.3 и ALPN h2")
			}
		}
		secure.Close()
		cancel()
		if err != nil {
			result.Error = "TLS-сертификат, имя SNI или TLS 1.3/h2 не прошли проверку"
			continue
		}
		result.Successes++
		result.Samples = append(result.Samples, Measurement{TCPMS: float64(tcpDuration.Microseconds()) / 1000, TLSMS: float64(tlsDuration.Microseconds()) / 1000})
	}
	if result.Successes == Samples {
		transport := &http.Transport{Proxy: nil, DialContext: checker.Guard.DialContext, TLSClientConfig: &tls.Config{ServerName: c.SNI, MinVersion: tls.VersionTLS13, RootCAs: checker.roots}, ForceAttemptHTTP2: true, DisableKeepAlives: true, DisableCompression: true, MaxResponseHeaderBytes: 16 << 10, TLSHandshakeTimeout: 3 * time.Second, ResponseHeaderTimeout: 3 * time.Second}
		client := &http.Client{Transport: transport, Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		request, e := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+c.Host+"/", nil)
		if e == nil {
			request.Host = c.SNI
			response, e := client.Do(request)
			if e == nil {
				result.HTTPStatus = response.StatusCode
				result.HTTPVerified = response.StatusCode >= 200 && response.StatusCode < 300 && response.ProtoMajor == 2
				response.Body.Close()
			}
		}
		transport.CloseIdleConnections()
		if !result.HTTPVerified {
			result.Error = "HTTP/h2 не подтвердил успешный ответ без перенаправления"
		}
	}
	result.Eligible = result.Successes == Samples && result.HTTPVerified
	if result.Eligible {
		result.Error = ""
	}
	durations := []float64{}
	for _, sample := range result.Samples {
		durations = append(durations, sample.TCPMS+sample.TLSMS)
	}
	sort.Float64s(durations)
	if len(durations) > 0 {
		result.MedianMS = durations[len(durations)/2]
		result.P95MS = durations[len(durations)-1]
	}
	return result
}
