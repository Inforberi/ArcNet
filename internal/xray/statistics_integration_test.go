package xray

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/arcvpn/arcvpn/internal/store"
)

// This fixture exercises the actual pinned Xray StatsService and real local
// bytes. Plain VLESS over loopback is intentional: this is not REALITY/VPS/DPI
// acceptance, and neither listener is exposed outside the test namespace.
func TestRealXrayStatistics(t *testing.T) {
	binaryPath := os.Getenv("ARCVPN_TEST_XRAY")
	if binaryPath == "" {
		t.Skip("set ARCVPN_TEST_XRAY to the pinned test binary")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	uid := "11111111-1111-4111-8111-111111111111"
	port := func() int {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		p := l.Addr().(*net.TCPAddr).Port
		l.Close()
		return p
	}
	serverPort, clientPort := port(), port()
	server := map[string]any{"log": map[string]any{"loglevel": "debug"}, "inbounds": []any{map[string]any{"listen": "127.0.0.1", "port": serverPort, "protocol": "vless", "streamSettings": map[string]any{"network": "tcp", "security": "none"}, "settings": map[string]any{"clients": []any{map[string]any{"id": uid, "email": "user-1@local"}}, "decryption": "none"}}}, "outbounds": []any{map[string]any{"protocol": "freedom", "settings": map[string]any{"finalRules": []any{map[string]any{"action": "allow", "ip": []string{"127.0.0.1/32"}}}}}}}
	controller := Controller{Path: filepath.Join(dir, "server.json"), Binary: binaryPath}
	jsonServer, _ := json.Marshal(server)
	jsonServer, err := controller.withStatistics(jsonServer)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(controller.Path, jsonServer, 0600); err != nil {
		t.Fatal(err)
	}
	client := map[string]any{"log": map[string]any{"loglevel": "debug"}, "inbounds": []any{map[string]any{"listen": "127.0.0.1", "port": clientPort, "protocol": "socks", "settings": map[string]any{"auth": "noauth"}}}, "outbounds": []any{map[string]any{"protocol": "vless", "streamSettings": map[string]any{"network": "tcp", "security": "none"}, "settings": map[string]any{"vnext": []any{map[string]any{"address": "127.0.0.1", "port": serverPort, "users": []any{map[string]any{"id": uid, "encryption": "none"}}}}}}}}
	jsonClient, _ := json.Marshal(client)
	clientPath := filepath.Join(dir, "client.json")
	if err := os.WriteFile(clientPath, jsonClient, 0600); err != nil {
		t.Fatal(err)
	}
	start := func(path string) {
		cmd := exec.Command("sh", "-c", `umask 077; exec "$@"`, "xray-fixture", binaryPath, "run", "-config", path)
		log, err := os.OpenFile(path+".log", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
		if err != nil {
			t.Fatal(err)
		}
		cmd.Stdout = log
		cmd.Stderr = log
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_ = cmd.Process.Signal(syscall.SIGTERM)
			done := make(chan struct{})
			go func() { _ = cmd.Wait(); close(done) }()
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				_ = cmd.Process.Kill()
				<-done
			}
			log.Close()
			if t.Failed() {
				blob, _ := os.ReadFile(path + ".log")
				if len(blob) > 4000 {
					blob = blob[len(blob)-4000:]
				}
				t.Log(string(blob))
			}
		})
	}
	start(controller.Path)
	start(clientPath)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(clientPort)), 50*time.Millisecond)
		if err == nil {
			conn.Close()
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, strings.Repeat("x", 128<<10)) }))
	defer upstream.Close()
	conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(clientPort)), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	_, err = conn.Write([]byte{5, 1, 0})
	if err != nil {
		t.Fatal(err)
	}
	reply := make([]byte, 2)
	if _, err = io.ReadFull(conn, reply); err != nil || reply[1] != 0 {
		t.Fatal("SOCKS greeting", err)
	}
	address := strings.TrimPrefix(upstream.URL, "http://")
	host, portText, _ := net.SplitHostPort(address)
	targetPort, _ := strconv.Atoi(portText)
	request := append([]byte{5, 1, 0, 1}, net.ParseIP(host).To4()...)
	request = binary.BigEndian.AppendUint16(request, uint16(targetPort))
	if _, err = conn.Write(request); err != nil {
		t.Fatal(err)
	}
	connectReply := make([]byte, 10)
	if _, err = io.ReadFull(conn, connectReply); err != nil || connectReply[1] != 0 {
		t.Fatal("SOCKS connect", err)
	}
	httpRequest, _ := http.NewRequest("GET", upstream.URL, nil)
	httpRequest.Close = true
	if err = httpRequest.Write(conn); err != nil {
		t.Fatal(err)
	}
	response, err := http.ReadResponse(bufio.NewReader(conn), httpRequest)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 129<<10))
	response.Body.Close()
	conn.Close()
	if err != nil || len(body) != 128<<10 {
		t.Fatal("fixture traffic", len(body), err)
	}
	var gotUpload, gotDownload int64
	for time.Now().Before(deadline) {
		items, _, err := controller.Statistics(context.Background(), []store.User{{ID: 1, UUID: uid}}, false)
		if err == nil && len(items) == 1 && items[0].Upload > 0 && items[0].Download >= 128<<10 {
			gotUpload = items[0].Upload
			gotDownload = items[0].Download
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if gotUpload == 0 || gotDownload < 128<<10 {
		t.Fatal("real StatsService did not report transferred bytes")
	}
	again, _, err := controller.Statistics(context.Background(), []store.User{{ID: 1, UUID: uid}}, false)
	if err != nil || again[0].Upload < gotUpload || again[0].Download < gotDownload {
		t.Fatal("query reset counters", err)
	}
	t.Logf("real loopback Xray counters: upload=%d download=%d; no REALITY/VPS acceptance", gotUpload, gotDownload)
}
