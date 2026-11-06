package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/arcvpn/arcvpn/internal/noderuntime"
	"github.com/arcvpn/arcvpn/internal/store"
	"github.com/arcvpn/arcvpn/internal/usage"
)

func TestRemoteUsageTwoNodesDeviceOwnershipReplayAndRevision(t *testing.T) {
	f := newCascadeUIFixture(t)
	ctx := context.Background()
	for _, n := range f.Nodes {
		if e := f.app.Store.Heartbeat(ctx, n.Secret, store.NodeHeartbeat{ProtocolVersion: 1, DeviceProtocol: 1, UsageProtocol: 1, XrayState: "active"}); e != nil {
			t.Fatal(e)
		}
	}
	if e := f.app.ChangeUserDevices(ctx, f.User.ID, 0, "Phone", "", nil); e != nil {
		t.Fatal(e)
	}
	reports := make([]usage.NodeReport, len(f.Nodes))
	for i, n := range f.Nodes {
		if e := f.app.ReconcileNodeCredentials(ctx, n.ID); e != nil {
			t.Fatal(e)
		}
		cmd, e := f.app.Store.PendingNodeCommand(ctx, n.ID, n.Secret)
		if e != nil || cmd == nil {
			t.Fatal("missing command", e)
		}
		if cmd.Kind != "apply_xray_usage_v1" {
			t.Fatal("missing accounting capability", cmd.Kind)
		}
		if e = noderuntime.ValidateRequest(noderuntime.ApplyRequest{ProtocolVersion: 1, Kind: cmd.Kind, OperationID: cmd.OperationID, Revision: cmd.Revision, SHA256: cmd.SHA256, Config: cmd.Config}); e != nil {
			t.Fatal(e)
		}
		clients, e := usage.NodeClients(cmd.Config)
		if e != nil || len(clients) != 1 {
			t.Fatal("device roster", e)
		}
		if e = f.app.Store.AcknowledgeNodeOperation(ctx, n.ID, n.Secret, cmd.OperationID, cmd.Revision, cmd.SHA256, "applied"); e != nil {
			t.Fatal(e)
		}
		r := usage.NodeReport{Version: 1, NodeID: n.ID, OperationID: cmd.OperationID, Revision: cmd.Revision, Digest: cmd.SHA256, Epoch: "node-process-a", StartedAt: 1, ObservedAt: time.Now().Unix() - 5}
		for name := range clients {
			r.Counters = append(r.Counters, usage.NamedCounter{Name: name, Upload: 100, Download: 200})
		}
		reports[i] = r
	}
	post := func(r usage.NodeReport, token string) int {
		t.Helper()
		b, e := json.Marshal(r)
		if e != nil {
			t.Fatal(e)
		}
		req := httptest.NewRequest(http.MethodPost, "http://localhost/api/nodes/usage", bytes.NewReader(b))
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		f.handler.ServeHTTP(w, req)
		return w.Code
	}
	if code := post(reports[0], f.Nodes[1].Secret); code != 401 {
		t.Fatal("cross-node report", code)
	}
	results := make(chan int, len(reports)*2)
	for i, r := range reports {
		go func() {
			for range 2 {
				results <- post(r, f.Nodes[i].Secret)
			}
		}()
	}
	for range len(reports) * 2 {
		if code := <-results; code != 204 {
			t.Fatal("concurrent report", code)
		}
	}
	assertTotal := func(want int64) {
		t.Helper()
		p, e := f.app.Store.Package(ctx, f.User.ID)
		if e != nil || p.Upload+p.Download != want {
			t.Fatal("total", p.Upload+p.Download, want, e)
		}
	}
	assertTotal(600)
	bad := reports[0]
	bad.Revision++
	if code := post(bad, f.Nodes[0].Secret); code != 409 {
		t.Fatal("unapplied revision", code)
	}
	bad = reports[0]
	bad.Counters = []usage.NamedCounter{{Name: "user-999@local", Upload: 900}}
	if code := post(bad, f.Nodes[0].Secret); code != 409 {
		t.Fatal("foreign user", code)
	}
	bad = reports[0]
	bad.ObservedAt = time.Now().Unix()
	bad.Counters = append([]usage.NamedCounter(nil), bad.Counters...)
	bad.Counters[0].Upload = 99
	if code := post(bad, f.Nodes[0].Secret); code != 409 {
		t.Fatal("regression", code)
	}
	assertTotal(600)
	local := usage.Counter{UserID: f.User.ID, Identity: f.User.UUID, Stream: "vless-reality", Epoch: "local-process", StartedAt: 1, ObservedAt: time.Now().Unix() - 5, Upload: 40}
	if e := f.app.Store.RecordUsage(ctx, []usage.Counter{local}, local.ObservedAt); e != nil {
		t.Fatal(e)
	}
	assertTotal(640)
	state, e := f.app.Store.UsageBackup(ctx)
	if e != nil || len(state.Checkpoints) != 3 {
		t.Fatal("portable checkpoints", len(state.Checkpoints), e)
	}
	if e = state.Validate(map[int64]string{f.User.ID: f.User.UUID}); e != nil {
		t.Fatal(e)
	}
	p, e := f.app.Store.Package(ctx, f.User.ID)
	if e != nil {
		t.Fatal(e)
	}
	if e = f.app.Store.SavePackage(ctx, f.User.ID, 0, 0, p.Version, true); e != nil {
		t.Fatal(e)
	}
	r := reports[0]
	r.ObservedAt = time.Now().Unix()
	r.Counters = append([]usage.NamedCounter(nil), r.Counters...)
	r.Counters[0].Upload += 25
	if code := post(r, f.Nodes[0].Secret); code != 204 {
		t.Fatal("new period report", code)
	}
	assertTotal(25)
	// Neither successful accounting nor a trusted report unlocks unsafe quotas.
	if e = f.app.Store.CheckPackageCoverage(ctx, 1000, 0); e != store.ErrUsageUnsupported {
		t.Fatal("quota guard removed", e)
	}
	if e = f.app.Store.DeleteRoute(ctx, f.Route.ID, f.Route.Version); e != nil {
		t.Fatal(e)
	}
	if e = f.app.Store.RemoveNode(ctx, f.Nodes[0].ID); e != nil {
		t.Fatal(e)
	}
	assertTotal(25)
	state, e = f.app.Store.UsageBackup(ctx)
	if e != nil || len(state.Checkpoints) != 2 {
		t.Fatal("retired source cleanup", len(state.Checkpoints), e)
	}
	for _, c := range state.Checkpoints {
		if c.Source == f.Nodes[0].ID {
			t.Fatal("retired checkpoint retained")
		}
	}
}
