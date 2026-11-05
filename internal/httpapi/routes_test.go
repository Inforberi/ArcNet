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
	"github.com/arcvpn/arcvpn/internal/routes"
	"github.com/arcvpn/arcvpn/internal/store"
)

func TestRouteAPIIsDraftOnlyAuthenticatedAndCSRFProtected(t *testing.T) {
	ctx := context.Background()
	database, e := db.Open(filepath.Join(t.TempDir(), "state.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer database.Close()
	st := &store.Store{DB: database}
	if e = st.Migrate(ctx); e != nil {
		t.Fatal(e)
	}
	secret := "Strong-admin-password-2026"
	hash, e := auth.HashPassword(secret)
	if e != nil {
		t.Fatal(e)
	}
	if e = st.CreateAdmin(ctx, "admin", hash); e != nil {
		t.Fatal(e)
	}
	handler := New(&app.App{Store: st}, slog.Default()).Routes()
	request := func(method, path string, body any, cookie *http.Cookie, csrf string) *httptest.ResponseRecorder {
		data, _ := json.Marshal(body)
		req := httptest.NewRequest(method, "https://203.0.113.4:8443"+path, bytes.NewReader(data))
		req.RemoteAddr = "127.0.0.1:34000"
		if cookie != nil {
			req.AddCookie(cookie)
		}
		if csrf != "" {
			req.Header.Set("X-CSRF-Token", csrf)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		return w
	}
	p := routes.DefaultPolicy()
	p.Name = "local direct"
	if w := request("POST", "/api/routes", p, nil, ""); w.Code != 401 {
		t.Fatalf("unauthorized create: %d", w.Code)
	}
	login := request("POST", "/api/login", map[string]string{"username": "admin", "password": secret}, nil, "")
	if login.Code != 200 {
		t.Fatal(login.Body.String())
	}
	var session struct {
		CSRF string `json:"csrf"`
	}
	if e = json.Unmarshal(login.Body.Bytes(), &session); e != nil {
		t.Fatal(e)
	}
	cookie := login.Result().Cookies()[0]
	if w := request("POST", "/api/routes", p, cookie, ""); w.Code != 403 {
		t.Fatalf("missing CSRF: %d", w.Code)
	}
	preview := request("POST", "/api/routes/preview", p, cookie, session.CSRF)
	if preview.Code != 200 || !strings.Contains(preview.Body.String(), `"side_effects":false`) {
		t.Fatalf("preview: %d %s", preview.Code, preview.Body.String())
	}
	empty := request("GET", "/api/routes", nil, cookie, "")
	if !strings.Contains(empty.Body.String(), `"items":[]`) {
		t.Fatal(empty.Body.String())
	}
	created := request("POST", "/api/routes", p, cookie, session.CSRF)
	if created.Code != 201 {
		t.Fatalf("create: %d %s", created.Code, created.Body.String())
	}
	var saved routes.Policy
	if e = json.Unmarshal(created.Body.Bytes(), &saved); e != nil {
		t.Fatal(e)
	}
	if saved.Version != 1 || saved.ID == "" {
		t.Fatal(saved)
	}
	if w := request("POST", "/api/routes/"+saved.ID+"/simulate", map[string]any{"samples": []map[string]any{{"at": 100, "health": map[string]string{"local": "healthy"}}}}, cookie, session.CSRF); w.Code != 200 || !strings.Contains(w.Body.String(), `"published":false`) {
		t.Fatalf("simulate: %d %s", w.Code, w.Body.String())
	}
	saved.Name = "renamed"
	if w := request("PUT", "/api/routes/"+saved.ID, map[string]any{"policy": saved, "expected_version": 1}, cookie, session.CSRF); w.Code != 200 {
		t.Fatalf("update: %d %s", w.Code, w.Body.String())
	}
	if w := request("DELETE", "/api/routes/"+saved.ID, map[string]int{"expected_version": 1}, cookie, session.CSRF); w.Code != 409 {
		t.Fatalf("stale delete: %d", w.Code)
	}
	if w := request("DELETE", "/api/routes/"+saved.ID, map[string]int{"expected_version": 2}, cookie, session.CSRF); w.Code != 200 {
		t.Fatalf("delete: %d", w.Code)
	}

	grant, e := st.CreateEnrollment(ctx, "Referenced exit")
	if e != nil {
		t.Fatal(e)
	}
	node, _, e := st.RegisterNode(ctx, grant)
	if e != nil {
		t.Fatal(e)
	}
	p.Name = "Protected cascade"
	p.Mode = "cascade"
	p.PrimaryExit = node.ID
	protected := request("POST", "/api/routes", p, cookie, session.CSRF)
	if protected.Code != 201 {
		t.Fatal(protected.Body.String())
	}
	if e = json.Unmarshal(protected.Body.Bytes(), &saved); e != nil {
		t.Fatal(e)
	}
	if w := request("DELETE", "/api/nodes/"+node.ID, nil, cookie, ""); w.Code != 403 {
		t.Fatal("node revoke without CSRF", w.Code)
	}
	if w := request("DELETE", "/api/nodes/"+node.ID, nil, cookie, session.CSRF); w.Code != 409 || !strings.Contains(w.Body.String(), p.Name) {
		t.Fatalf("route dependency not reported: %d %s", w.Code, w.Body.String())
	}
	if w := request("DELETE", "/api/routes/"+saved.ID, map[string]int64{"expected_version": saved.Version}, cookie, session.CSRF); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if w := request("DELETE", "/api/nodes/"+node.ID, nil, cookie, session.CSRF); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
}
