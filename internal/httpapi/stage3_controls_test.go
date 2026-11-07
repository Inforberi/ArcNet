package httpapi

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/arcvpn/arcvpn/internal/app"
	"github.com/arcvpn/arcvpn/internal/auth"
	"github.com/arcvpn/arcvpn/internal/db"
	"github.com/arcvpn/arcvpn/internal/store"
)

func TestFirewallReadStatusDoesNotPromisePortReachability(t *testing.T) {
	s, cookie := monitoringFixture(t)
	socket := filepath.Join(t.TempDir(), "firewall.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	s.App.Core.ControlSocket = socket
	command := make(chan string, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		line, _ := bufio.NewReader(conn).ReadString('\n')
		command <- line
		_, _ = conn.Write([]byte(`{"active":true,"rules":[{"port":3443,"transport":"tcp"}]}`))
	}()
	r := httptest.NewRequest("GET", "https://example.test/api/settings/firewall", nil)
	r.AddCookie(cookie)
	w := httptest.NewRecorder()
	s.Routes().ServeHTTP(w, r)
	var state struct {
		Available bool `json:"available"`
		Active    bool `json:"active"`
		Writable  bool `json:"writable"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &state); err != nil || w.Code != 200 || !state.Available || !state.Active || state.Writable {
		t.Fatal("invalid read-only state", w.Code, w.Body.String(), err)
	}
	if got := <-command; got != "firewall status\n" {
		t.Fatal("unexpected privileged operation", got)
	}
}

func TestStage3AuthenticatedOperationsFailClosed(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ARCVPN_DATA_DIR", dir)
	database, e := db.Open(filepath.Join(dir, "state.sqlite"))
	if e != nil {
		t.Fatal(e)
	}
	defer database.Close()
	st := &store.Store{DB: database}
	ctx := context.Background()
	if e = st.Migrate(ctx); e != nil {
		t.Fatal(e)
	}
	token, csrf := "test-secret-session-token", "test-csrf"
	if e = st.AddSession(ctx, auth.HashToken(token), csrf, time.Now().Add(time.Hour).Unix()); e != nil {
		t.Fatal(e)
	}
	router := New(&app.App{Store: st}, slog.Default()).Routes()
	request := func(method, path string, payload any, contentType, csrfHeader string, authenticated bool) *httptest.ResponseRecorder {
		var body []byte
		if payload != nil {
			switch val := payload.(type) {
			case []byte:
				body = val
			default:
				body, _ = json.Marshal(val)
			}
		}
		r := httptest.NewRequest(method, "http://example.test"+path, bytes.NewReader(body))
		if authenticated {
			r.AddCookie(&http.Cookie{Name: "arcvpn_session", Value: token})
		}
		if csrfHeader != "" {
			r.Header.Set("X-CSRF-Token", csrfHeader)
		}
		if contentType != "" {
			r.Header.Set("Content-Type", contentType)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	if got := request("GET", "/api/settings/firewall", nil, "", "", false); got.Code != 401 {
		t.Fatal("firewall leaked without session", got.Code)
	}
	if got := request("POST", "/api/settings/firewall", map[string]any{"port": 50000, "transport": "udp", "allow": true}, "application/json", "", true); got.Code != 403 {
		t.Fatal("CSRF bypass", got.Code)
	}
	if got := request("POST", "/api/settings/firewall", map[string]any{"port": 22, "transport": "tcp", "allow": false}, "application/json", csrf, true); got.Code != 400 {
		t.Fatal("SSH port mutation accepted", got.Code)
	}
	if got := request("POST", "/api/settings/firewall", map[string]any{"port": 50000, "transport": "udp", "allow": true}, "application/json", csrf, true); got.Code != 409 {
		t.Fatal("unconfigured port opened", got.Code)
	}
	if got := request("GET", "/api/settings/firewall", nil, "", "", true); got.Code != 200 || !bytes.Contains(got.Body.Bytes(), []byte(`"available":false`)) {
		t.Fatal("invalid local firewall status", got.Code, got.Body.String())
	}
	if got := request("POST", "/api/system/update/upload", []byte("unsigned archive"), "application/octet-stream", csrf, true); got.Code != 409 {
		t.Fatal("upload accepted without trusted helper", got.Code, got.Body.String())
	}
	if _, e = os.Stat(updateStagePath()); !os.IsNotExist(e) {
		t.Fatal("unverified archive left staged", e)
	}
	if got := request("POST", "/api/system/update/apply", map[string]string{"confirm": "WRONG"}, "application/json", csrf, true); got.Code != 409 {
		t.Fatal("apply accepted without trusted helper", got.Code)
	}
}
