package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/arcvpn/arcvpn/internal/app"
	"github.com/arcvpn/arcvpn/internal/auth"
	"github.com/arcvpn/arcvpn/internal/db"
	"github.com/arcvpn/arcvpn/internal/noderuntime"
	"github.com/arcvpn/arcvpn/internal/routes"
	"github.com/arcvpn/arcvpn/internal/store"
	"github.com/arcvpn/arcvpn/internal/xray"
)

type cascadeUINode struct {
	ID       string `json:"id"`
	Secret   string `json:"secret"`
	Baseline []byte `json:"-"`
}
type cascadeUIFixture struct {
	app     *app.App
	server  *Server
	handler http.Handler
	Session string          `json:"session"`
	CSRF    string          `json:"csrf"`
	Route   routes.Policy   `json:"route"`
	Nodes   []cascadeUINode `json:"nodes"`
	User    store.User      `json:"-"`
}

func newCascadeUIFixture(t *testing.T) *cascadeUIFixture {
	t.Helper()
	ctx := context.Background()
	database, e := db.Open(filepath.Join(t.TempDir(), "panel.sqlite"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = database.Close() })
	st := &store.Store{DB: database}
	if e := st.Migrate(ctx); e != nil {
		t.Fatal(e)
	}
	if e := st.CreateAdmin(ctx, "admin", "fixture-unused-hash"); e != nil {
		t.Fatal(e)
	}
	session, e := auth.Secret(32)
	if e != nil {
		t.Fatal(e)
	}
	csrf, e := auth.Secret(32)
	if e != nil {
		t.Fatal(e)
	}
	if e := st.AddSession(ctx, auth.HashToken(session), csrf, time.Now().Add(time.Hour).Unix()); e != nil {
		t.Fatal(e)
	}
	userID, e := auth.UUID()
	if e != nil {
		t.Fatal(e)
	}
	token, e := auth.Secret(32)
	if e != nil {
		t.Fatal(e)
	}
	if e := st.InsertUser(ctx, store.User{Name: "Existing client", UUID: userID, Subscription: token, Enabled: true, CreatedAt: time.Now().Unix()}); e != nil {
		t.Fatal(e)
	}
	user, e := st.ByToken(ctx, token)
	if e != nil {
		t.Fatal(e)
	}
	f := &cascadeUIFixture{app: &app.App{Store: st}, Session: session, CSRF: csrf, User: user}
	for index, ip := range []string{"9.9.9.9", "1.1.1.1"} {
		grant, e := st.CreateEnrollmentForNode(ctx, []string{"Entry A", "Exit B"}[index], ip, 443)
		if e != nil {
			t.Fatal(e)
		}
		node, secret, e := st.RegisterNode(ctx, grant)
		if e != nil {
			t.Fatal(e)
		}
		if e := st.Heartbeat(ctx, secret, store.NodeHeartbeat{ProtocolVersion: 1, CascadeProtocol: 1, XrayState: "active"}); e != nil {
			t.Fatal(e)
		}
		key, e := xray.NewCascadeChannel(15443)
		if e != nil {
			t.Fatal(e)
		}
		profile := store.Reality{Address: ip, Port: 443, Target: "www.example.com", SNI: "www.example.com", Private: key.Private, Public: key.Public, ShortID: key.ShortID, Enabled: true}
		if e := st.PutNodeRealityIfAbsent(ctx, node.ID, profile); e != nil {
			t.Fatal(e)
		}
		baseline, e := xray.Generate(profile, []store.User{user})
		if e != nil {
			t.Fatal(e)
		}
		if _, e := st.QueueNodeDeployment(ctx, node.ID, baseline); e != nil {
			t.Fatal(e)
		}
		command, e := st.PendingNodeCommand(ctx, node.ID, secret)
		if e != nil || command == nil {
			t.Fatal("baseline poll", e)
		}
		if e := st.AcknowledgeNodeOperation(ctx, node.ID, secret, command.OperationID, command.Revision, command.SHA256, "applied"); e != nil {
			t.Fatal(e)
		}
		f.Nodes = append(f.Nodes, cascadeUINode{ID: node.ID, Secret: secret, Baseline: baseline})
	}
	p := routes.DefaultPolicy()
	p.Name = "Transparent cascade"
	p.Mode = "cascade"
	p.EntryNode = f.Nodes[0].ID
	p.PrimaryExit = f.Nodes[1].ID
	f.Route, e = st.SaveRoute(ctx, p, 0)
	if e != nil {
		t.Fatal(e)
	}
	f.server = New(f.app, slog.New(slog.NewTextHandler(io.Discard, nil)))
	f.handler = f.server.Routes()
	return f
}
func (f *cascadeUIFixture) request(method, path string, body any, authenticated, csrf bool) *httptest.ResponseRecorder {
	data, _ := json.Marshal(body)
	r := httptest.NewRequest(method, "http://localhost"+path, bytes.NewReader(data))
	r.RemoteAddr = "127.0.0.1:20001"
	if authenticated {
		r.AddCookie(&http.Cookie{Name: "arcvpn_session", Value: f.Session})
	}
	if csrf {
		r.Header.Set("X-CSRF-Token", f.CSRF)
	}
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, r)
	return w
}
func TestCascadeUIAuthenticatedPreviewApplyRollback(t *testing.T) {
	runCascadeUIAuthenticatedPreviewApplyRollback(t, newCascadeUIFixture(t))
}

func TestCascadeUIWithUsageNodesPreservesPollAndRollback(t *testing.T) {
	f := newCascadeUIFixture(t)
	ctx := context.Background()
	for i, n := range f.Nodes {
		if e := f.app.Store.Heartbeat(ctx, n.Secret, store.NodeHeartbeat{ProtocolVersion: 1, DeviceProtocol: 1, UsageProtocol: 1, CascadeProtocol: 1, XrayState: "active"}); e != nil {
			t.Fatal(e)
		}
		if _, e := f.app.DeployNode(ctx, n.ID); e != nil {
			t.Fatal(e)
		}
		cmd, e := f.app.Store.PendingNodeCommand(ctx, n.ID, n.Secret)
		if e != nil || cmd == nil || cmd.Kind != "apply_xray_usage_v1" {
			t.Fatal("usage preparation", e)
		}
		if e = f.app.Store.AcknowledgeNodeOperation(ctx, n.ID, n.Secret, cmd.OperationID, cmd.Revision, cmd.SHA256, "applied"); e != nil {
			t.Fatal(e)
		}
		f.Nodes[i].Baseline = cmd.Config
	}
	runCascadeUIAuthenticatedPreviewApplyRollback(t, f)
	for _, n := range f.Nodes {
		capable, configured, e := f.app.Store.NodeUsageStatus(ctx, n.ID)
		if e != nil || !capable || !configured {
			t.Fatal("accounting was not restored", e)
		}
	}
}

func runCascadeUIAuthenticatedPreviewApplyRollback(t *testing.T, f *cascadeUIFixture) {
	t.Helper()
	ctx := context.Background()
	path := "/api/routes/" + f.Route.ID
	for _, target := range []struct{ method, path string }{{"GET", "/api/cascade-deployments"}, {"POST", path + "/deployment-preview"}, {"POST", path + "/deployments"}, {"POST", "/api/cascade-deployments/unknown/start"}, {"POST", "/api/cascade-deployments/unknown/rollback"}, {"DELETE", "/api/cascade-deployments/unknown"}} {
		if w := f.request(target.method, target.path, nil, false, false); w.Code != 401 {
			t.Fatal("auth missing", w.Code)
		}
		if target.method != "GET" {
			if w := f.request(target.method, target.path, nil, true, false); w.Code != 403 {
				t.Fatal("CSRF missing", w.Code)
			}
		}
	}
	preview := f.request("POST", path+"/deployment-preview", map[string]any{"expected_version": f.Route.Version, "channel_port": 15443}, true, true)
	if preview.Code != 200 {
		t.Fatal("preview failed", preview.Code)
	}
	var plan app.CascadePreview
	if e := json.Unmarshal(preview.Body.Bytes(), &plan); e != nil {
		t.Fatal(e)
	}
	if items, e := f.app.Store.CascadeDeployments(ctx); e != nil || len(items) != 0 {
		t.Fatal("preview mutated DB")
	}
	apply := map[string]any{"expected_version": f.Route.Version, "channel_port": 15443, "preview_token": plan.Token, "confirm": "APPLY_CASCADE"}
	apply["channel_port"] = 15444
	if w := f.request("POST", path+"/deployments", apply, true, true); w.Code != 409 {
		t.Fatal("changed plan accepted", w.Code)
	}
	apply["channel_port"] = 15443
	apply["confirm"] = ""
	if w := f.request("POST", path+"/deployments", apply, true, true); w.Code != 400 {
		t.Fatal("missing confirmation accepted", w.Code)
	}
	apply["confirm"] = "APPLY_CASCADE"
	response := f.request("POST", path+"/deployments", apply, true, true)
	if response.Code != 202 {
		t.Fatal("apply failed", response.Code)
	}
	var deployment store.CascadeDeployment
	_ = json.Unmarshal(response.Body.Bytes(), &deployment)
	for _, index := range []int{1, 0} {
		node := f.Nodes[index]
		if e := f.app.ReconcileNodeCredentials(ctx, node.ID); e != nil {
			t.Fatal("collector interfered with cascade", e)
		}
		command, e := f.app.Store.PendingNodeCommand(ctx, node.ID, node.Secret)
		if e != nil || command == nil {
			t.Fatal("staged command absent", e)
		}
		if e := noderuntime.ValidateRequest(noderuntime.ApplyRequest{ProtocolVersion: command.ProtocolVersion, Kind: command.Kind, OperationID: command.OperationID, Revision: command.Revision, SHA256: command.SHA256, Config: command.Config}); e != nil {
			t.Fatal("helper rejected generated command", e)
		}
		var before, after struct {
			Inbounds []json.RawMessage `json:"inbounds"`
		}
		_ = json.Unmarshal(node.Baseline, &before)
		_ = json.Unmarshal(command.Config, &after)
		var a, b any
		_ = json.Unmarshal(before.Inbounds[0], &a)
		_ = json.Unmarshal(after.Inbounds[0], &b)
		if !reflect.DeepEqual(a, b) {
			t.Fatal("client listener or credentials changed")
		}
		if e := f.app.Store.AcknowledgeNodeOperation(ctx, node.ID, node.Secret, command.OperationID, command.Revision, command.SHA256, "applied"); e != nil {
			t.Fatal(e)
		}
	}
	user, e := f.app.Store.ByToken(ctx, f.User.Subscription)
	if e != nil || user.UUID != f.User.UUID || user.Subscription != f.User.Subscription {
		t.Fatal("client profile changed")
	}
	listing := f.request("GET", "/api/cascade-deployments", nil, true, false)
	if listing.Code != 200 || listing.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("private list", listing.Code)
	}
	var items struct {
		Items []store.CascadeView `json:"items"`
	}
	_ = json.Unmarshal(listing.Body.Bytes(), &items)
	if len(items.Items) != 1 || items.Items[0].State != "awaiting_probe" || items.Items[0].Binding == nil || items.Items[0].Binding.EndpointID == "" {
		t.Fatal("binding/status missing")
	}
	for _, secret := range []string{f.Nodes[0].Secret, f.Nodes[1].Secret, f.User.UUID, f.User.Subscription, "private_key", "config_ciphertext", "secret_ciphertext"} {
		if strings.Contains(listing.Body.String(), secret) || strings.Contains(preview.Body.String(), secret) {
			t.Fatal("private detail leaked")
		}
	}
	d := items.Items[0]
	rollback := f.request("POST", "/api/cascade-deployments/"+deployment.ID+"/rollback", map[string]any{"expected_version": d.Version, "confirm": "ROLLBACK_CASCADE"}, true, true)
	if rollback.Code != 202 {
		t.Fatal("rollback failed", rollback.Code)
	}
	for _, node := range f.Nodes {
		command, e := f.app.Store.PendingNodeCommand(ctx, node.ID, node.Secret)
		if e != nil || command == nil {
			t.Fatal("rollback poll", e)
		}
		if e := f.app.Store.AcknowledgeNodeOperation(ctx, node.ID, node.Secret, command.OperationID, command.Revision, command.SHA256, "applied"); e != nil {
			t.Fatal(e)
		}
	}
	d2, e := f.app.Store.CascadeDeployment(ctx, deployment.ID)
	if e != nil || d2.State != "rolled_back" {
		t.Fatal("rollback state", e)
	}
	deleted := f.request("DELETE", "/api/cascade-deployments/"+deployment.ID, map[string]any{"expected_version": d2.Version, "confirm": "DELETE_CASCADE_JOURNAL"}, true, true)
	if deleted.Code != 200 {
		t.Fatal("journal cleanup", deleted.Code)
	}
}

// Opt-in real React/API fixture; no fixture-only endpoint enters production.
func TestCascadeUIBrowserFixture(t *testing.T) {
	dir := os.Getenv("ARCVPN_CASCADE_UI_FIXTURE")
	if dir == "" {
		t.Skip("opt-in browser fixture")
	}
	f := newCascadeUIFixture(t)
	if e := f.server.WithWebRoot("/workspace/frontend/dist"); e != nil {
		t.Fatal(e)
	}
	f.handler = f.server.Routes()
	data, _ := json.Marshal(f)
	if e := os.WriteFile(filepath.Join(dir, "fixture.json"), data, 0600); e != nil {
		t.Fatal(e)
	}
	done := make(chan struct{})
	var once sync.Once
	mux := http.NewServeMux()
	mux.Handle("/", f.handler)
	mux.HandleFunc("POST /__fixture_done", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204); once.Do(func() { close(done) }) })
	listener, e := net.Listen("tcp", "0.0.0.0:38567")
	if e != nil {
		t.Fatal(e)
	}
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })
	select {
	case <-done:
	case <-time.After(90 * time.Second):
		t.Fatal("browser fixture timed out")
	}
}
