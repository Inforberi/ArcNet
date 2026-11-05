package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/arcvpn/arcvpn/internal/app"
	"github.com/arcvpn/arcvpn/internal/auth"
	"github.com/arcvpn/arcvpn/internal/db"
	"github.com/arcvpn/arcvpn/internal/store"
)

func TestExternalProbeGrantCSRFAndReportReplay(t *testing.T) {
	ctx := context.Background()
	dbh, err := db.Open(filepath.Join(t.TempDir(), "probe.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer dbh.Close()
	st := &store.Store{DB: dbh}
	if err = st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	hash, err := auth.HashPassword("a-strong-password-for-api-probe-tests")
	if err != nil {
		t.Fatal(err)
	}
	if err = st.CreateAdmin(ctx, "admin", hash); err != nil {
		t.Fatal(err)
	}
	enrollment, err := st.CreateEnrollmentForNode(ctx, "NL-test", "9.9.9.9", 443)
	if err != nil {
		t.Fatal(err)
	}
	node, secret, err := st.RegisterNode(ctx, enrollment)
	if err != nil {
		t.Fatal(err)
	}
	if err = st.Heartbeat(ctx, secret, store.NodeHeartbeat{Version: "0.6.3-stage4", ProtocolVersion: 1, XrayState: "active"}); err != nil {
		t.Fatal(err)
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
	svc := New(&app.App{Store: st}, slog.Default())
	routes := svc.Routes()
	call := func(method, path string, payload any, cookie *http.Cookie, csrf, bearer string) *httptest.ResponseRecorder {
		var body []byte
		if payload != nil {
			body, _ = json.Marshal(payload)
		}
		req := httptest.NewRequest(method, "https://arcvpn.example"+path, bytes.NewReader(body))
		req.RemoteAddr = "127.0.0.1:13001"
		if cookie != nil {
			req.AddCookie(cookie)
		}
		if csrf != "" {
			req.Header.Set("X-CSRF-Token", csrf)
		}
		if bearer != "" {
			req.Header.Set("Authorization", bearer)
		}
		recorder := httptest.NewRecorder()
		routes.ServeHTTP(recorder, req)
		return recorder
	}
	grantPath := "/api/nodes/" + node.ID + "/probe/grant"
	if got := call("POST", grantPath, map[string]any{}, nil, "", ""); got.Code != 401 {
		t.Fatalf("unauth grant: %d", got.Code)
	}
	login := call("POST", "/api/login", map[string]string{"username": "admin", "password": "a-strong-password-for-api-probe-tests"}, nil, "", "")
	if login.Code != 200 {
		t.Fatalf("login: %s", login.Body.String())
	}
	cookies := login.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("missing cookie")
	}
	var session struct {
		CSRF string `json:"csrf"`
	}
	if err = json.Unmarshal(login.Body.Bytes(), &session); err != nil || session.CSRF == "" {
		t.Fatal("missing CSRF")
	}
	if got := call("POST", grantPath, map[string]any{}, cookies[0], "", ""); got.Code != 403 {
		t.Fatalf("CSRF bypass: %d", got.Code)
	}
	created := call("POST", grantPath, map[string]any{}, cookies[0], session.CSRF, "")
	if created.Code != 201 {
		t.Fatalf("grant: %d %s", created.Code, created.Body.String())
	}
	var grant store.ProbeGrant
	if err = json.Unmarshal(created.Body.Bytes(), &grant); err != nil || len(grant.Token) < 40 {
		t.Fatalf("missing grant: %+v %v", grant, err)
	}
	report := store.ProbeReport{NodeID: node.ID, Revision: op.Revision, Digest: op.Digest, Result: "pass", DurationMS: 1000}
	reportPath := "/api/nodes/probe/report"
	if got := call("POST", reportPath, report, nil, "", "Probe wrong"); got.Code != 401 {
		t.Fatalf("invalid token accepted: %d", got.Code)
	}
	if got := call("POST", reportPath, report, nil, "", "Bearer "+grant.Token); got.Code != 401 {
		t.Fatalf("wrong scheme accepted: %d", got.Code)
	}
	bad := report
	bad.Digest = "bad"
	if got := call("POST", reportPath, bad, nil, "", "Probe "+grant.Token); got.Code != 401 {
		t.Fatalf("bad report accepted: %d", got.Code)
	}
	passed := call("POST", reportPath, report, nil, "", "Probe "+grant.Token)
	if passed.Code != 200 {
		t.Fatalf("report: %d %s", passed.Code, passed.Body.String())
	}
	if got := call("POST", reportPath, report, nil, "", "Probe "+grant.Token); got.Code != 401 {
		t.Fatalf("replay accepted: %d", got.Code)
	}
	listed := call("GET", "/api/nodes", nil, cookies[0], "", "")
	if listed.Code != 200 {
		t.Fatalf("list: %d", listed.Code)
	}
	control, err := st.NodeControl(ctx, node.ID)
	if err != nil || control.Probe != "pass" || control.Published {
		t.Fatalf("wrong result state: %+v %v", control, err)
	}
	// The next revision MUST invalidate the previously accepted result.
	if _, err = st.QueueNodeDeployment(ctx, node.ID, []byte(`{"inbounds":[{"tag":"new"}]}`)); err != nil {
		t.Fatal(err)
	}
	control, err = st.NodeControl(ctx, node.ID)
	if err != nil || control.Probe != "unknown" {
		t.Fatalf("old probe promoted revision: %+v %v", control, err)
	}
}
