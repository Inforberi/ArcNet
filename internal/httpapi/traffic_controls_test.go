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
	"github.com/arcvpn/arcvpn/internal/routingprofile"
	"github.com/arcvpn/arcvpn/internal/store"
)

func TestTrafficControlsAuthCSRFImportAndConcurrency(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	st := &store.Store{DB: database}
	ctx := context.Background()
	if err = st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	password := "Strong-admin-password-2026"
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	if err = st.CreateAdmin(ctx, "admin", hash); err != nil {
		t.Fatal(err)
	}
	handler := New(&app.App{Store: st}, slog.Default()).Routes()
	var cookie *http.Cookie
	csrf := ""
	request := func(method, path string, body any, authenticated, protected bool) *httptest.ResponseRecorder {
		blob, _ := json.Marshal(body)
		r := httptest.NewRequest(method, "https://203.0.113.4:8443"+path, bytes.NewReader(blob))
		r.RemoteAddr = "127.0.0.1:34000"
		if authenticated && cookie != nil {
			r.AddCookie(cookie)
		}
		if protected {
			r.Header.Set("X-CSRF-Token", csrf)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	p := routingprofile.Default()
	p.Name = "Правила"
	p.Enabled = true
	p.Direct = []string{"example.com"}
	body := map[string]any{"profile": p, "expected_version": 0}
	for _, path := range []string{"/api/routing-profiles", "/api/targets/catalog", "/api/targets/nodes", "/api/targets/checks/unknown"} {
		if w := request("GET", path, nil, false, false); w.Code != 401 {
			t.Fatalf("anonymous %s: %d", path, w.Code)
		}
	}
	login := request("POST", "/api/login", map[string]string{"username": "admin", "password": password}, false, false)
	if login.Code != 200 {
		t.Fatal(login.Body.String())
	}
	cookie = login.Result().Cookies()[0]
	var session struct {
		CSRF string `json:"csrf"`
	}
	_ = json.Unmarshal(login.Body.Bytes(), &session)
	csrf = session.CSRF
	for _, path := range []string{"/api/routing-profiles", "/api/routing-profiles/import", "/api/routing-profiles/list-preview", "/api/targets/check", "/api/targets/catalog"} {
		if w := request("POST", path, body, true, false); w.Code != 403 {
			t.Fatalf("CSRF %s: %d", path, w.Code)
		}
	}
	created := request("POST", "/api/routing-profiles", body, true, true)
	if created.Code != 201 {
		t.Fatal(created.Body.String())
	}
	var saved routingprofile.Profile
	_ = json.Unmarshal(created.Body.Bytes(), &saved)
	saved.Name = "New name"
	body = map[string]any{"profile": saved, "expected_version": saved.Version}
	if w := request("PUT", "/api/routing-profiles/"+saved.ID, body, true, true); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if w := request("PUT", "/api/routing-profiles/"+saved.ID, body, true, true); w.Code != 409 {
		t.Fatal("lost update accepted")
	}
	exported := request("GET", "/api/routing-profiles/"+saved.ID+"/export", nil, true, false)
	if exported.Code != 200 {
		t.Fatal(exported.Body.String())
	}
	imported := request("POST", "/api/routing-profiles/import", map[string]string{"archive": exported.Body.String()}, true, true)
	if imported.Code != 200 || !strings.Contains(imported.Body.String(), `"side_effects":false`) {
		t.Fatal(imported.Body.String())
	}
	if profiles, _ := st.RoutingProfiles(ctx); len(profiles) != 1 {
		t.Fatal("preview wrote database")
	}
	if w := request("POST", "/api/routing-profiles/list-preview", map[string]string{"text": "https://example.com"}, true, true); w.Code != 400 {
		t.Fatal("invalid list accepted")
	}
	if w := request("POST", "/api/routing-profiles/list-preview", map[string]string{"text": strings.Repeat("x", 600000)}, true, true); w.Code != 400 {
		t.Fatal("unbounded import")
	}
	if w := request("POST", "/api/targets/catalog", map[string]any{"url": "https://example.com/catalog", "expected_version": 1}, true, true); w.Code != 400 {
		t.Fatal("download accepted without independently pinned signer")
	}
	if w := request("POST", "/api/targets/check", map[string]any{"node_id": "local", "candidates": []map[string]string{{"host": "metadata.internal"}}}, true, true); w.Code != 400 {
		t.Fatal("SSRF target accepted")
	}
	if w := request("GET", "/api/nodes/target-checks/poll?node_id=local", nil, false, false); w.Code != 401 {
		t.Fatal("remote caller claimed local probe identity")
	}
	if w := request("POST", "/api/nodes/target-checks/report", map[string]string{"node_id": "local"}, false, false); w.Code != 401 {
		t.Fatal("anonymous local report accepted")
	}
}
