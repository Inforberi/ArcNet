package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arcvpn/arcvpn/internal/app"
	"github.com/arcvpn/arcvpn/internal/auth"
	"github.com/arcvpn/arcvpn/internal/db"
	"github.com/arcvpn/arcvpn/internal/store"
	"github.com/arcvpn/arcvpn/internal/xray"
)

func TestStage4APIPollRevisionAndPermissions(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	database, e := db.Open(filepath.Join(dir, "panel.sqlite"))
	if e != nil {
		t.Fatal(e)
	}
	defer database.Close()
	state := &store.Store{DB: database}
	if e = state.Migrate(ctx); e != nil {
		t.Fatal(e)
	}
	passwordHash, e := auth.HashPassword("valid-password-2026-very-long")
	if e != nil {
		t.Fatal(e)
	}
	if e = state.CreateAdmin(ctx, "admin", passwordHash); e != nil {
		t.Fatal(e)
	}
	reality := store.Reality{Address: "8.8.8.8", Port: 443, Target: "www.example.com", SNI: "www.example.com", Private: strings.Repeat("A", 43), Public: strings.Repeat("B", 43), ShortID: "0123456789abcdef", Enabled: true}
	if e = state.SaveReality(ctx, reality); e != nil {
		t.Fatal(e)
	}
	if e = state.InsertUser(ctx, store.User{Name: "Client", UUID: "c9cdf017-50e4-45ef-9a20-7a9b3cf7bc8b", Enabled: true, CreatedAt: 1, Subscription: "s"}); e != nil {
		t.Fatal(e)
	}
	stub := filepath.Join(dir, "xray")
	if e = os.WriteFile(stub, []byte("#!/bin/sh\necho 'PrivateKey: "+strings.Repeat("C", 43)+"'\necho 'Password: "+strings.Repeat("D", 43)+"'\n"), 0700); e != nil {
		t.Fatal(e)
	}
	server := New(&app.App{Store: state, Core: xray.Controller{Binary: stub}}, slog.Default())
	if e = server.WithPublicURL("https://vpn.example"); e != nil {
		t.Fatal(e)
	}
	handler := server.Routes()
	request := func(method, path string, body any, cookie *http.Cookie, csrf, agentToken string) *httptest.ResponseRecorder {
		b, _ := json.Marshal(body)
		r := httptest.NewRequest(method, "https://vpn.example"+path, bytes.NewReader(b))
		r.RemoteAddr = "127.0.0.1:4444"
		if cookie != nil {
			r.AddCookie(cookie)
		}
		if csrf != "" {
			r.Header.Set("X-CSRF-Token", csrf)
		}
		if agentToken != "" {
			r.Header.Set("Authorization", "Bearer "+agentToken)
		}
		if body != nil {
			r.Header.Set("Content-Type", "application/json")
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	login := request("POST", "/api/login", map[string]string{"username": "admin", "password": "valid-password-2026-very-long"}, nil, "", "")
	if login.Code != 200 {
		t.Fatal(login.Body.String())
	}
	var session struct {
		CSRF string `json:"csrf"`
	}
	_ = json.Unmarshal(login.Body.Bytes(), &session)
	cookie := login.Result().Cookies()[0]
	issued := request("POST", "/api/nodes/enroll", map[string]any{"name": "NL", "address": "9.9.9.9", "port": 443}, cookie, session.CSRF, "")
	if issued.Code != 201 {
		t.Fatalf("enroll: %s", issued.Body.String())
	}
	var enrollment struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(issued.Body.Bytes(), &enrollment)
	registered := request("POST", "/api/nodes/register", map[string]string{"token": enrollment.Token}, nil, "", "")
	if registered.Code != 201 {
		t.Fatalf("register: %s", registered.Body.String())
	}
	var identity struct {
		ID     string `json:"node_id"`
		Secret string `json:"secret"`
	}
	_ = json.Unmarshal(registered.Body.Bytes(), &identity)
	if denied := request("POST", "/api/nodes/"+identity.ID+"/deploy", nil, cookie, "", ""); denied.Code != 403 {
		t.Fatalf("no CSRF accepted: %d", denied.Code)
	}
	if denied := request("POST", "/api/nodes/"+identity.ID+"/deploy", nil, nil, "", ""); denied.Code != 401 {
		t.Fatalf("no login accepted: %d", denied.Code)
	}
	if early := request("GET", "/api/nodes/"+identity.ID+"/links", nil, cookie, "", ""); early.Code != 409 {
		t.Fatalf("links published before apply confirmation: %d", early.Code)
	}
	start := request("POST", "/api/nodes/"+identity.ID+"/deploy", nil, cookie, session.CSRF, "")
	if start.Code != 202 {
		t.Fatalf("deploy: %d %s", start.Code, start.Body.String())
	}
	var op store.NodeOperation
	_ = json.Unmarshal(start.Body.Bytes(), &op)
	if op.Revision != 1 {
		t.Fatal("initial revision must be 1")
	}
	if denied := request("GET", "/api/nodes/poll?node_id="+identity.ID, nil, nil, "", "incorrect"); denied.Code != 401 {
		t.Fatal("unauthorized node received command")
	}
	beforeHello := request("GET", "/api/nodes/poll?node_id="+identity.ID, nil, nil, "", identity.Secret)
	if beforeHello.Code != 409 {
		t.Fatalf("unknown agent protocol received config: %d", beforeHello.Code)
	}
	oldHello := request("POST", "/api/nodes/heartbeat", map[string]any{"version": "0.3.0", "protocol_version": 0, "xray_state": "unknown"}, nil, "", identity.Secret)
	if oldHello.Code != 204 {
		t.Fatalf("legacy heartbeat rejected: %d", oldHello.Code)
	}
	if oldPoll := request("GET", "/api/nodes/poll?node_id="+identity.ID, nil, nil, "", identity.Secret); oldPoll.Code != 409 {
		t.Fatalf("legacy agent received config: %d", oldPoll.Code)
	}
	newHello := request("POST", "/api/nodes/heartbeat", map[string]any{"version": "0.6.0-stage4", "protocol_version": 1, "xray_state": "unknown"}, nil, "", identity.Secret)
	if newHello.Code != 204 {
		t.Fatalf("compatible heartbeat rejected: %d %s", newHello.Code, newHello.Body.String())
	}
	poll := request("GET", "/api/nodes/poll?node_id="+identity.ID, nil, nil, "", identity.Secret)
	if poll.Code != 200 {
		t.Fatalf("poll: %d %s", poll.Code, poll.Body.String())
	}
	var command store.NodeCommand
	_ = json.Unmarshal(poll.Body.Bytes(), &command)
	if command.Kind != "apply_xray_v1" || command.OperationID != op.ID || command.SHA256 != op.Digest {
		t.Fatal("controller sent unbound operation")
	}
	invalid := request("POST", "/api/nodes/ack", map[string]any{"node_id": identity.ID, "operation_id": op.ID, "revision": op.Revision, "sha256": strings.Repeat("0", 64), "result": "applied"}, nil, "", identity.Secret)
	if invalid.Code != 409 {
		t.Fatal("bad ack accepted")
	}
	ack := request("POST", "/api/nodes/ack", map[string]any{"node_id": identity.ID, "operation_id": op.ID, "revision": op.Revision, "sha256": op.Digest, "result": "applied"}, nil, "", identity.Secret)
	if ack.Code != 204 {
		t.Fatalf("ack: %d %s", ack.Code, ack.Body.String())
	}
	links := request("GET", "/api/nodes/"+identity.ID+"/links", nil, cookie, "", "")
	if links.Code != 200 || !strings.Contains(links.Body.String(), "vless://") || strings.Contains(links.Body.String(), strings.Repeat("C", 43)) {
		t.Fatalf("admin cannot obtain safe direct test link: %d %s", links.Code, links.Body.String())
	}
	idle := request("GET", "/api/nodes/poll?node_id="+identity.ID, nil, nil, "", identity.Secret)
	if idle.Code != 204 {
		t.Fatalf("applied command redelivered: %d", idle.Code)
	}
	list := request("GET", "/api/nodes", nil, cookie, "", "")
	if list.Code != 200 || !strings.Contains(list.Body.String(), `"revision_status":"applied"`) {
		t.Fatalf("no applied revision: %s", list.Body.String())
	}
	if strings.Contains(list.Body.String(), strings.Repeat("C", 43)) || strings.Contains(list.Body.String(), strings.Repeat("D", 43)) {
		t.Fatal("private Reality keys leaked to UI")
	}
}
func TestStage4SSHInstallFailsClosedWithoutSignedRelease(t *testing.T) {
	ctx := context.Background()
	d, e := db.Open(filepath.Join(t.TempDir(), "state.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer d.Close()
	s := &store.Store{DB: d}
	if e = s.Migrate(ctx); e != nil {
		t.Fatal(e)
	}
	handler := New(&app.App{Store: s}, slog.Default()).Routes()
	r := httptest.NewRequest("POST", "https://vpn.example/api/nodes/ssh/install", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatalf("anonymous SSH installation allowed: %d", w.Code)
	}
}
