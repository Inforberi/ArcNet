package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/arcvpn/arcvpn/internal/app"
	"github.com/arcvpn/arcvpn/internal/store"
)

func TestCascadeProofAPIPublishesStableExistingClientProfile(t *testing.T) {
	f := newCascadeUIFixture(t)
	ctx := context.Background()
	f.server.publicURL = "https://panel.example"
	plan, e := f.app.CascadePreview(ctx, f.Route.ID, f.Route.Version, 15443)
	if e != nil {
		t.Fatal(e)
	}
	d, e := f.app.ApplyCascadeFromPreview(ctx, f.Route.ID, f.Route.Version, 15443, plan.Token)
	if e != nil {
		t.Fatal(e)
	}
	for _, i := range []int{1, 0} {
		node := f.Nodes[i]
		c, e := f.app.Store.PendingNodeCommand(ctx, node.ID, node.Secret)
		if e != nil || c == nil {
			t.Fatal(e)
		}
		if e := f.app.Store.AcknowledgeNodeOperation(ctx, node.ID, node.Secret, c.OperationID, c.Revision, c.SHA256, "applied"); e != nil {
			t.Fatal(e)
		}
	}
	d, e = f.app.Store.CascadeDeployment(ctx, d.ID)
	if e != nil {
		t.Fatal(e)
	}
	path := "/api/cascade-deployments/" + d.ID + "/probe-grant"
	body := map[string]any{"expected_version": d.Version, "confirm": "CREATE_EXTERNAL_PROBE"}
	if w := f.request("POST", path, body, false, false); w.Code != 401 {
		t.Fatal("grant auth")
	}
	if w := f.request("POST", path, body, true, false); w.Code != 403 {
		t.Fatal("grant CSRF")
	}
	response := f.request("POST", path, body, true, true)
	if response.Code != 201 {
		t.Fatal("grant", response.Code)
	}
	var grant app.CascadeProbeBundle
	if e := json.Unmarshal(response.Body.Bytes(), &grant); e != nil {
		t.Fatal(e)
	}
	if grant.Controller != "https://panel.example" || grant.Link == "" {
		t.Fatal("runner bundle incomplete")
	}
	report := store.CascadeProbeReport{DeploymentID: d.ID, Version: grant.Version, EntryRevision: grant.EntryRevision, EntryDigest: grant.EntryDigest, ExitRevision: grant.ExitRevision, ExitDigest: grant.ExitDigest, ExitIP: grant.ExpectedIP, DNSMode: "doh_via_cascade", Result: "pass", DurationMS: 100}
	raw, _ := json.Marshal(report)
	r := httptest.NewRequest(http.MethodPost, "http://localhost/api/cascade-probes/report", bytes.NewReader(raw))
	r.Header.Set("Authorization", "CascadeProbe "+grant.Token)
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatal("report", w.Code)
	}
	d, e = f.app.Store.CascadeDeployment(ctx, d.ID)
	if e != nil {
		t.Fatal(e)
	}
	publish := map[string]any{"expected_version": d.Version, "confirm": "PUBLISH_CASCADE"}
	if w := f.request("POST", "/api/cascade-deployments/"+d.ID+"/publish", publish, true, false); w.Code != 403 {
		t.Fatal("publish CSRF")
	}
	if w := f.request("POST", "/api/cascade-deployments/"+d.ID+"/publish", publish, true, true); w.Code != 200 {
		t.Fatal("publish", w.Code)
	}
	links, e := f.app.SubscriptionLinks(ctx, f.User)
	if e != nil || len(links) != 1 || links[0] != grant.Link {
		t.Fatal("stable entry not added to existing subscription", e)
	}
	user, e := f.app.Store.ByToken(ctx, f.User.Subscription)
	if e != nil || user.UUID != f.User.UUID {
		t.Fatal("existing client identity changed")
	}
}
