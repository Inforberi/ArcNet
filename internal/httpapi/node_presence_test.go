package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/arcvpn/arcvpn/internal/noderuntime"
	"github.com/arcvpn/arcvpn/internal/store"
	"github.com/arcvpn/arcvpn/internal/usage"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNodePresenceIsSeparateFromBillingAndBoundToNodeAndRevision(t *testing.T) {
	f := newCascadeUIFixture(t)
	ctx := context.Background()
	reports := []usage.NodePresenceReport{}
	for i, node := range f.Nodes {
		if err := f.app.Store.Heartbeat(ctx, node.Secret, store.NodeHeartbeat{ProtocolVersion: 1, DeviceProtocol: 1, UsageProtocol: 1, PresenceProtocol: 1, XrayState: "active"}); err != nil {
			t.Fatal(err)
		}
		if err := f.app.ReconcileNodeCredentials(ctx, node.ID); err != nil {
			t.Fatal(err)
		}
		command, err := f.app.Store.PendingNodeCommand(ctx, node.ID, node.Secret)
		if err != nil || command == nil {
			t.Fatal(err)
		}
		if !usage.NodePresenceEnabled(command.Config) {
			t.Fatal("presence capability was not negotiated")
		}
		if err = noderuntime.ValidateRequest(noderuntime.ApplyRequest{ProtocolVersion: 1, Kind: command.Kind, OperationID: command.OperationID, Revision: command.Revision, SHA256: command.SHA256, Config: command.Config}); err != nil {
			t.Fatal(err)
		}
		if err = f.app.Store.AcknowledgeNodeOperation(ctx, node.ID, node.Secret, command.OperationID, command.Revision, command.SHA256, "applied"); err != nil {
			t.Fatal(err)
		}
		clients, err := usage.NodeClients(command.Config)
		if err != nil {
			t.Fatal(err)
		}
		report := usage.NodePresenceReport{Version: 1, NodeID: node.ID, OperationID: command.OperationID, Revision: command.Revision, Digest: command.SHA256, Epoch: "presence-epoch", StartedAt: 1, ObservedAt: time.Now().Unix()}
		for name := range clients {
			value := i == 0
			report.Clients = append(report.Clients, usage.NamedPresence{Name: name, Online: &value})
		}
		reports = append(reports, report)
	}
	sources, err := f.app.Store.NodePresenceSources(ctx)
	if err != nil {
		t.Fatal(err)
	}
	f.app.UserPresence.Roster([]usage.PresenceUser{{ID: f.User.ID, Identity: f.User.UUID, Sources: sources}})
	post := func(report usage.NodePresenceReport, secret string) int {
		raw, _ := json.Marshal(report)
		r := httptest.NewRequest(http.MethodPost, "https://example.test/api/nodes/presence", bytes.NewReader(raw))
		r.Header.Set("Authorization", "Bearer "+secret)
		w := httptest.NewRecorder()
		f.handler.ServeHTTP(w, r)
		return w.Code
	}
	if code := post(reports[0], f.Nodes[1].Secret); code != 401 {
		t.Fatal("cross-node presence", code)
	}
	before, err := f.app.Store.Package(ctx, f.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	for i, report := range reports {
		if code := post(report, f.Nodes[i].Secret); code != 204 {
			t.Fatal("valid report", code)
		}
	}
	items, summary := f.app.UserPresence.Snapshot([]usage.TrafficUser{{ID: f.User.ID, Identity: f.User.UUID}}, time.Now().Unix())
	if items[f.User.ID].State != "online" || summary.Online != 1 || len(items[f.User.ID].Sources) != len(f.Nodes) {
		t.Fatal("presence aggregation", items, summary)
	}
	for _, source := range items[f.User.ID].Sources {
		if source.NodeID == f.Nodes[1].ID && source.State != "offline" {
			t.Fatal("online leaked to another node", source)
		}
	}
	after, err := f.app.Store.Package(ctx, f.User.ID)
	if err != nil || before.Upload != after.Upload || before.Download != after.Download {
		t.Fatal("presence affected billing", err)
	}
	bad := reports[0]
	bad.Revision++
	if code := post(bad, f.Nodes[0].Secret); code != 409 {
		t.Fatal("unapplied revision", code)
	}
	bad = reports[0]
	bad.ObservedAt -= usage.FreshFor + 1
	if code := post(bad, f.Nodes[0].Secret); code != 409 {
		t.Fatal("old report", code)
	}
	bad = reports[0]
	bad.Clients = []usage.NamedPresence{{Name: "user-999@local"}}
	if code := post(bad, f.Nodes[0].Secret); code != 409 {
		t.Fatal("missing online/foreign ownership", code)
	}
	if p, _ := f.app.UserPresence.Snapshot([]usage.TrafficUser{{ID: f.User.ID, Identity: f.User.UUID}}, time.Now().Unix()+usage.FreshFor+1); p[f.User.ID].State != "stale" {
		t.Fatal("offline inferred from expired report")
	}
}
