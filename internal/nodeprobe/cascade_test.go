package nodeprobe

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"net/http"
	"strings"
	"testing"
)

type cascadeRoundTrip func(*http.Request) (*http.Response, error)

func (f cascadeRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestCascadeChecksEgressAndDNSOverProvidedTunnelTransport(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: cascadeRoundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		data := []byte(`{"ip":"1.1.1.1"}`)
		content := "application/json"
		if calls == 1 {
			if r.URL.String() != "https://api.ipify.org?format=json" {
				t.Fatal("unfixed egress target")
			}
		} else {
			if r.URL.String() != "https://1.1.1.1/dns-query" || r.Method != "POST" || r.Header.Get("Content-Type") != "application/dns-message" {
				t.Fatal("unexpected DNS transport")
			}
			query, _ := io.ReadAll(r.Body)
			data = append([]byte{}, query...)
			data[2] = 0x81
			data[3] = 0x80
			binary.BigEndian.PutUint16(data[6:8], 1)
			data = append(data, 0xc0, 0x0c, 0, 1, 0, 1, 0, 0, 0, 60, 0, 4, 93, 184, 216, 34)
			content = "application/dns-message"
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{content}}, Body: io.NopCloser(bytes.NewReader(data))}, nil
	})}
	result, e := checkCascade(context.Background(), client)
	if e != nil || result.ExitIP != "1.1.1.1" || result.DNSMode != CascadeDNSMode || calls != 2 {
		t.Fatal("cascade checks", e)
	}
}
func TestCascadeRejectsPrivateEgressAndMalformedDNS(t *testing.T) {
	client := &http.Client{Transport: cascadeRoundTrip(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"ip":"127.0.0.1"}`))}, nil
	})}
	if _, e := checkCascade(context.Background(), client); e == nil {
		t.Fatal("private egress accepted")
	}
	query := []byte{1, 2, 1, 0, 0, 1, 0, 0, 0, 0, 0, 0, 7, 'e', 'x', 'a', 'm', 'p', 'l', 'e', 3, 'c', 'o', 'm', 0, 0, 1, 0, 1}
	if validDNSAnswer(query, query) || validDNSAnswer(query, []byte{1, 2}) {
		t.Fatal("query or truncated answer accepted")
	}
}
