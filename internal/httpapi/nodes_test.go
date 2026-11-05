package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arcvpn/arcvpn/internal/app"
	"github.com/arcvpn/arcvpn/internal/auth"
	"github.com/arcvpn/arcvpn/internal/db"
	"github.com/arcvpn/arcvpn/internal/store"
)

func TestNodeEnrollmentAPI(t *testing.T) {
	ctx := context.Background()
	d, err := db.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	st := &store.Store{DB: d}
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	hash, _ := auth.HashPassword("Strong-admin-password-123456")
	if err := st.CreateAdmin(ctx, "admin", hash); err != nil {
		t.Fatal(err)
	}
	svc := New(&app.App{Store: st}, slog.Default())
	if err := svc.WithPublicURL("https://203.0.113.4:8443"); err != nil {
		t.Fatal(err)
	}
	h := svc.Routes()
	request := func(method, path string, body any, cookie *http.Cookie, csrf, token string) *httptest.ResponseRecorder {
		b, _ := json.Marshal(body)
		r := httptest.NewRequest(method, "https://203.0.113.4:8443"+path, bytes.NewReader(b))
		r.RemoteAddr = "127.0.0.1:3333"
		if cookie != nil {
			r.AddCookie(cookie)
		}
		if csrf != "" {
			r.Header.Set("X-CSRF-Token", csrf)
		}
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	login := request("POST", "/api/login", map[string]string{"username": "admin", "password": "Strong-admin-password-123456"}, nil, "", "")
	if login.Code != 200 {
		t.Fatalf("login: %s", login.Body.String())
	}
	var session struct {
		CSRF string `json:"csrf"`
	}
	_ = json.Unmarshal(login.Body.Bytes(), &session)
	cookies := login.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatal("session cookie missing")
	}
	if r := request("POST", "/api/nodes/enroll", map[string]string{"name": "NL-1"}, cookies[0], "", ""); r.Code != 403 {
		t.Fatalf("no csrf: %d", r.Code)
	}
	issued := request("POST", "/api/nodes/enroll", map[string]string{"name": "NL-1"}, cookies[0], session.CSRF, "")
	if issued.Code != 201 {
		t.Fatalf("grant: %s", issued.Body.String())
	}
	var grant struct {
		Token      string `json:"token"`
		Controller string `json:"controller"`
	}
	_ = json.Unmarshal(issued.Body.Bytes(), &grant)
	if grant.Controller != "https://203.0.113.4:8443" || len(grant.Token) < 40 {
		t.Fatalf("grant: %+v", grant)
	}
	unauthorizedBinary := request("GET", "/node/binary?checksum=1", nil, nil, "", "")
	if unauthorizedBinary.Code != 401 {
		t.Fatalf("binary must require enrollment: %d", unauthorizedBinary.Code)
	}
	registered := request("POST", "/api/nodes/register", map[string]string{"token": grant.Token}, nil, "", "")
	if registered.Code != 201 {
		t.Fatalf("register %s", registered.Body.String())
	}
	var node struct {
		NodeID string `json:"node_id"`
		Secret string `json:"secret"`
	}
	_ = json.Unmarshal(registered.Body.Bytes(), &node)
	if r := request("POST", "/api/nodes/register", map[string]string{"token": grant.Token}, nil, "", ""); r.Code != 403 {
		t.Fatal("replay accepted")
	}
	invalid := request("POST", "/api/nodes/heartbeat", map[string]any{"xray_state": "active"}, nil, "", "badtoken")
	if invalid.Code != 401 {
		t.Fatalf("unauth heartbeat: %d", invalid.Code)
	}
	live := request("POST", "/api/nodes/heartbeat", map[string]any{"xray_state": "active", "version": "0.3.0", "memory_total": 1024, "memory_available": 512, "load_1": 0.3, "uptime_seconds": 12}, nil, "", node.Secret)
	if live.Code != 204 {
		t.Fatalf("heartbeat: %d %s", live.Code, live.Body.String())
	}
	listed := request("GET", "/api/nodes", nil, cookies[0], "", "")
	if listed.Code != 200 || !strings.Contains(listed.Body.String(), "NL-1") || !strings.Contains(listed.Body.String(), `"online":true`) {
		t.Fatalf("list: %d %s", listed.Code, listed.Body.String())
	}
	revoked := request("DELETE", "/api/nodes/"+node.NodeID, nil, cookies[0], session.CSRF, "")
	if revoked.Code != 200 {
		t.Fatalf("revoke: %d", revoked.Code)
	}
	if r := request("POST", "/api/nodes/heartbeat", map[string]any{"xray_state": "active"}, nil, "", node.Secret); r.Code != 401 {
		t.Fatal("revoked node heartbeat accepted")
	}
}
