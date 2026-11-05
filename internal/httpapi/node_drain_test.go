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

func TestNodeDrainIsAuthorizedCSRFProtectedAndDoesNotStopXray(t *testing.T) {
	ctx := context.Background()
	database, err := db.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	st := &store.Store{DB: database}
	if err = st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	hashed, err := auth.HashPassword("very-long-administrator-password-2026")
	if err != nil {
		t.Fatal(err)
	}
	if err = st.CreateAdmin(ctx, "admin", hashed); err != nil {
		t.Fatal(err)
	}
	grant, err := st.CreateEnrollmentForNode(ctx, "NL-1", "9.9.9.9", 443)
	if err != nil {
		t.Fatal(err)
	}
	node, secret, err := st.RegisterNode(ctx, grant)
	if err != nil {
		t.Fatal(err)
	}
	if err = st.Heartbeat(ctx, secret, store.NodeHeartbeat{Version: "0.6.1-stage4", ProtocolVersion: 1, XrayState: "active"}); err != nil {
		t.Fatal(err)
	}
	svc := New(&app.App{Store: st}, slog.Default())
	if err = svc.WithPublicURL("https://203.0.113.4:8443"); err != nil {
		t.Fatal(err)
	}
	handler := svc.Routes()
	request := func(method, path string, body any, cookie *http.Cookie, csrf string) *httptest.ResponseRecorder {
		b, _ := json.Marshal(body)
		req := httptest.NewRequest(method, "https://203.0.113.4:8443"+path, bytes.NewReader(b))
		req.RemoteAddr = "127.0.0.1:34000"
		if cookie != nil {
			req.AddCookie(cookie)
		}
		if csrf != "" {
			req.Header.Set("X-CSRF-Token", csrf)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		return response
	}
	path := "/api/nodes/" + node.ID + "/drain"
	if got := request("POST", path, map[string]bool{"draining": true}, nil, ""); got.Code != 401 {
		t.Fatalf("anonymous drain was not rejected: %d", got.Code)
	}
	login := request("POST", "/api/login", map[string]string{"username": "admin", "password": "very-long-administrator-password-2026"}, nil, "")
	if login.Code != 200 {
		t.Fatalf("login failed %d %s", login.Code, login.Body.String())
	}
	cookies := login.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatal("no authenticated cookie")
	}
	var session struct {
		CSRF string `json:"csrf"`
	}
	if err = json.Unmarshal(login.Body.Bytes(), &session); err != nil || session.CSRF == "" {
		t.Fatal("no csrf")
	}
	if got := request("POST", path, map[string]bool{"draining": true}, cookies[0], ""); got.Code != 403 {
		t.Fatalf("drain without CSRF: %d", got.Code)
	}
	if got := request("POST", path, map[string]any{}, cookies[0], session.CSRF); got.Code != 400 {
		t.Fatalf("missing draining bool: %d", got.Code)
	}
	drained := request("POST", path, map[string]bool{"draining": true}, cookies[0], session.CSRF)
	if drained.Code != 200 || !strings.Contains(drained.Body.String(), `"draining":true`) {
		t.Fatalf("failed to drain: %d %s", drained.Code, drained.Body.String())
	}
	if got := request("GET", "/api/nodes/"+node.ID+"/links", nil, cookies[0], ""); got.Code != 409 || !strings.Contains(got.Body.String(), "drain") {
		t.Fatalf("new links not suppressed: %d %s", got.Code, got.Body.String())
	}
	if got := request("POST", path, map[string]bool{"draining": false}, cookies[0], session.CSRF); got.Code != 409 {
		t.Fatalf("unconfigured node resumed: %d", got.Code)
	}
	// A working agent can still poll; drain is not revocation or shutdown.
	if err = st.Heartbeat(ctx, secret, store.NodeHeartbeat{Version: "0.6.1-stage4", ProtocolVersion: 1, XrayState: "active"}); err != nil {
		t.Fatalf("drain broke agent: %v", err)
	}
	op, err := st.QueueNodeDeployment(ctx, node.ID, []byte(`{"inbounds":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = st.PendingNodeCommand(ctx, node.ID, secret); err != nil {
		t.Fatal(err)
	}
	if err = st.AcknowledgeNodeOperation(ctx, node.ID, secret, op.ID, op.Revision, op.Digest, "applied"); err != nil {
		t.Fatal(err)
	}
	resumed := request("POST", path, map[string]bool{"draining": false}, cookies[0], session.CSRF)
	if resumed.Code != 200 || !strings.Contains(resumed.Body.String(), `"draining":false`) {
		t.Fatalf("resume failed: %d %s", resumed.Code, resumed.Body.String())
	}
	state, err := st.NodeControl(ctx, node.ID)
	if err != nil || state.Probe != "unknown" || state.Published {
		t.Fatalf("drain/resume changed VPN evidence: %+v %v", state, err)
	}
}
