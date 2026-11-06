package xray_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Validate the unmodified production REALITY configs with a pinned real Xray.
// The loopback transport fixture then changes ONLY listen/address/transports
// and private-destination restrictions; this is not a REALITY handshake test.
func TestRealCascadeForwardingAndClosedFailure(t *testing.T) {
	binary := os.Getenv("ARCVPN_TEST_XRAY")
	if binary == "" {
		t.Skip("set ARCVPN_TEST_XRAY to a pinned Xray binary")
	}
	left, right, _, _, _ := cascadePair(t)
	dir := t.TempDir()
	write := func(name string, data []byte) string {
		path := filepath.Join(dir, name+".json")
		if e := os.WriteFile(path, data, 0600); e != nil {
			t.Fatal(e)
		}
		return path
	}
	for name, raw := range map[string][]byte{"production-entry": left, "production-exit": right} {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		diagnostic, err := exec.CommandContext(ctx, binary, "run", "-test", "-config", write(name, raw)).CombinedOutput()
		cancel()
		if err != nil {
			safe := regexp.MustCompile(`[A-Za-z0-9_-]{32,}`).ReplaceAllString(string(diagnostic), "[redacted]")
			t.Fatal(name, "pinned Xray config validation failed", err, safe)
		}
	}
	port := func() int {
		l, e := net.Listen("tcp", "127.0.0.1:0")
		if e != nil {
			t.Fatal(e)
		}
		defer l.Close()
		return l.Addr().(*net.TCPAddr).Port
	}
	aPort, bPort, cPort := port(), port(), port()
	var hits atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = io.WriteString(w, "through-two-nodes")
	}))
	defer upstream.Close()
	var a, b map[string]any
	_ = json.Unmarshal(left, &a)
	_ = json.Unmarshal(right, &b)
	plainInbound := func(in map[string]any, p int) {
		in["listen"] = "127.0.0.1"
		in["port"] = p
		in["streamSettings"] = map[string]any{"network": "tcp", "security": "none"}
		for _, cl := range in["settings"].(map[string]any)["clients"].([]any) {
			delete(cl.(map[string]any), "flow")
		}
	}
	plainInbound(a["inbounds"].([]any)[0].(map[string]any), aPort)
	plainInbound(b["inbounds"].([]any)[1].(map[string]any), bPort)
	// Keep only the machine listener in the isolated exit fixture.
	b["inbounds"] = []any{b["inbounds"].([]any)[1]}
	out := a["outbounds"].([]any)[1].(map[string]any)
	settings := out["settings"].(map[string]any)
	settings["address"] = "127.0.0.1"
	settings["port"] = bPort
	delete(settings, "flow")
	out["streamSettings"] = map[string]any{"network": "tcp", "security": "none"}
	for _, cfg := range []map[string]any{a, b} {
		routing := cfg["routing"].(map[string]any)
		routing["rules"] = routing["rules"].([]any)[1:]
	}
	// New Xray freedom defaults block loopback; permit only the fixture target.
	b["outbounds"].([]any)[1].(map[string]any)["settings"] = map[string]any{"finalRules": []any{map[string]any{"action": "allow", "ip": []string{"127.0.0.1/32"}}}}
	client := map[string]any{"log": map[string]any{"loglevel": "error"}, "inbounds": []any{map[string]any{"listen": "127.0.0.1", "port": cPort, "protocol": "http", "settings": map[string]any{}}}, "outbounds": []any{map[string]any{"protocol": "vless", "settings": map[string]any{"address": "127.0.0.1", "port": aPort, "id": "11111111-1111-4111-8111-111111111111", "encryption": "none"}, "streamSettings": map[string]any{"network": "tcp", "security": "none"}}}}
	start := func(name string, cfg map[string]any, p int) *exec.Cmd {
		raw, _ := json.Marshal(cfg)
		path := write(name, raw)
		cmd := exec.Command(binary, "run", "-config", path)
		// Discard Xray errors: generated configs contain machine credentials.
		cmd.Stdout = io.Discard
		cmd.Stderr = io.Discard
		if e := cmd.Start(); e != nil {
			t.Fatal(e)
		}
		t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			c, e := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(p)), 50*time.Millisecond)
			if e == nil {
				c.Close()
				return cmd
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatal("Xray fixture listener did not start: " + name)
		return nil
	}
	exit := start("exit", b, bPort)
	start("entry", a, aPort)
	start("client", client, cPort)
	proxy, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", cPort))
	transport := &http.Transport{Proxy: http.ProxyURL(proxy), DisableKeepAlives: true}
	defer transport.CloseIdleConnections()
	browser := &http.Client{Transport: transport, Timeout: 2 * time.Second}
	response, e := browser.Get(upstream.URL)
	if e != nil {
		t.Fatal("two-hop request", e)
	}
	body, e := io.ReadAll(response.Body)
	response.Body.Close()
	if e != nil || strings.TrimSpace(string(body)) != "through-two-nodes" || hits.Load() != 1 {
		t.Fatal("two-hop response mismatch")
	}
	if e := exit.Process.Kill(); e != nil {
		t.Fatal(e)
	}
	if response, e := browser.Get(upstream.URL); e == nil {
		response.Body.Close()
		t.Fatal("request succeeded after exit failure")
	}
	if hits.Load() != 1 {
		t.Fatal("entry bypassed failed exit")
	}
	t.Log("unmodified REALITY config checks + loopback two-hop bytes/fail-closed PASS; not VPS/REALITY handshake acceptance")
}
