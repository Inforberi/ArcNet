package xray

import (
	"context"
	"github.com/arcvpn/arcvpn/internal/store"
	"github.com/arcvpn/arcvpn/internal/testfixture"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestRealXrayPresenceConnectedIdleClosedAndUnavailable(t *testing.T) {
	binary := os.Getenv("ARCVPN_TEST_XRAY")
	if binary == "" {
		t.Skip("requires pinned real Xray and isolated non-loopback network")
	}
	addresses, err := net.InterfaceAddrs()
	if err != nil {
		t.Fatal(err)
	}
	host := ""
	for _, address := range addresses {
		if ip, _, e := net.ParseCIDR(address.String()); e == nil && ip.To4() != nil && !ip.IsLoopback() {
			host = ip.String()
			break
		}
	}
	if host == "" {
		t.Fatal("isolated non-loopback client address required; loopback is excluded by Xray")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	uid := "11111111-1111-4111-8111-111111111111"
	port := testfixture.Port(t, host)
	proxy := testfixture.Port(t, "127.0.0.1")
	controller := Controller{Binary: binary, Path: filepath.Join(dir, "xray.json")}
	users := []store.User{{ID: 1, UUID: uid, Enabled: true}}
	config := map[string]any{"log": map[string]any{"loglevel": "none"}, "inbounds": []any{map[string]any{"listen": host, "port": port, "protocol": "vless", "settings": map[string]any{"decryption": "none", "clients": []any{map[string]any{"id": uid, "email": "user-1@local"}}}, "streamSettings": map[string]any{"network": "tcp", "security": "none"}}}, "outbounds": []any{map[string]any{"protocol": "freedom", "settings": map[string]any{"finalRules": []any{map[string]any{"action": "allow", "ip": []string{"127.0.0.1/32"}}}}}}}
	testfixture.WriteJSON(t, controller.Path, config)
	raw, _ := os.ReadFile(controller.Path)
	raw, err = controller.withStatistics(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(controller.Path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	clientPath := filepath.Join(dir, "client.json")
	testfixture.WriteJSON(t, clientPath, map[string]any{"log": map[string]any{"loglevel": "none"}, "inbounds": []any{map[string]any{"listen": "127.0.0.1", "port": proxy, "protocol": "socks", "settings": map[string]any{"auth": "noauth"}}}, "outbounds": []any{map[string]any{"protocol": "vless", "settings": map[string]any{"vnext": []any{map[string]any{"address": host, "port": port, "users": []any{map[string]any{"id": uid, "encryption": "none"}}}}}, "streamSettings": map[string]any{"network": "tcp", "security": "none"}}}})
	server := testfixture.Start(t, binary, "run", "-config", controller.Path)
	testfixture.Start(t, binary, "run", "-config", clientPath)
	testfixture.Wait(t, "client SOCKS", func() bool {
		c, e := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(proxy)), 50*time.Millisecond)
		if e == nil {
			c.Close()
		}
		return e == nil
	})
	var lastQueryError error
	t.Cleanup(func() {
		if t.Failed() {
			t.Log("last guarded presence query:", lastQueryError)
		}
	})
	check := func(want bool) bool {
		rows, e := controller.Presence(context.Background(), users)
		lastQueryError = e
		return e == nil && len(rows) == 1 && rows[0].Online != nil && *rows[0].Online == want
	}
	testfixture.Wait(t, "initial confirmed offline", func() bool { return check(false) })
	stream := testfixture.SOCKS(t, proxy, testfixture.Echo(t))
	testfixture.Transfer(t, stream)
	testfixture.Wait(t, "real authenticated connection online", func() bool { return check(true) })
	// Idle means no application traffic. Keep the authenticated stream open.
	time.Sleep(21 * time.Second)
	if !check(true) {
		t.Fatal("idle authenticated connection lost online status")
	}
	stream.Close()
	testfixture.Wait(t, "closed connection offline", func() bool { return check(false) })
	server.Stop()
	if _, err = controller.Presence(context.Background(), users); err == nil {
		t.Fatal("dead source returned confirmed offline")
	}
	t.Log("real Xray non-loopback: offline -> transfer/online -> idle online -> closed/offline -> source unavailable; plain VLESS, not REALITY/VPS acceptance")
}
