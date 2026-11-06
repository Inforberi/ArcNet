package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/arcvpn/arcvpn/internal/noderuntime"
	"github.com/arcvpn/arcvpn/internal/store"
)

func TestRemoteDeviceAccessWaitsForEveryNodeAndRevokeConverges(t *testing.T) {
	f := newCascadeUIFixture(t)
	ctx := context.Background()
	for _, node := range f.Nodes {
		if e := f.app.Store.Heartbeat(ctx, node.Secret, store.NodeHeartbeat{ProtocolVersion: 1, CascadeProtocol: 1, DeviceProtocol: 1, XrayState: "active"}); e != nil {
			t.Fatal(e)
		}
	}
	if e := f.app.ChangeUserDevices(ctx, f.User.ID, 0, "TV", "", nil); e != nil {
		t.Fatal(e)
	}
	record, e := f.app.Store.UserDevices(ctx, f.User.ID)
	if e != nil {
		t.Fatal(e)
	}
	device := record.Devices[0]
	if _, e := f.app.DeviceLinks(ctx, f.User.ID, device.ID); !errors.Is(e, store.ErrRemoteCredentialsPending) {
		t.Fatal("new key issued before ACKs", e)
	}
	poll := func(index int) *store.NodeCommand {
		t.Helper()
		node := f.Nodes[index]
		r := httptest.NewRequest(http.MethodGet, "http://localhost/api/nodes/poll?node_id="+node.ID, nil)
		r.Header.Set("Authorization", "Bearer "+node.Secret)
		w := httptest.NewRecorder()
		f.handler.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatal("credential poll", w.Code)
		}
		var command store.NodeCommand
		if json.Unmarshal(w.Body.Bytes(), &command) != nil {
			t.Fatal("decode command")
		}
		if e := noderuntime.ValidateRequest(noderuntime.ApplyRequest{ProtocolVersion: command.ProtocolVersion, Kind: command.Kind, OperationID: command.OperationID, Revision: command.Revision, SHA256: command.SHA256, Config: command.Config}); e != nil {
			t.Fatal("device helper config", e)
		}
		return &command
	}
	ack := func(index int, command *store.NodeCommand) {
		t.Helper()
		node := f.Nodes[index]
		if e := f.app.Store.AcknowledgeNodeOperation(ctx, node.ID, node.Secret, command.OperationID, command.Revision, command.SHA256, "applied"); e != nil {
			t.Fatal(e)
		}
	}
	for i := range f.Nodes {
		command := poll(i)
		if command.Kind != "apply_xray_devices_v1" || strings.Contains(string(command.Config), f.User.UUID) || !strings.Contains(string(command.Config), device.UUID) {
			t.Fatal("retired common key or missing device key")
		}
		ack(i, command)
		if i == 0 {
			if _, e := f.app.DeviceLinks(ctx, f.User.ID, device.ID); !errors.Is(e, store.ErrRemoteCredentialsPending) {
				t.Fatal("issued after only one ACK", e)
			}
		}
	}
	links, e := f.app.DeviceLinks(ctx, f.User.ID, device.ID)
	if e != nil || len(links) != 2 {
		t.Fatal("remote direct device links missing", e)
	}
	for _, link := range links {
		if !strings.Contains(link, device.UUID) {
			t.Fatal("wrong device credential")
		}
	}
	// Limit metadata changes cause no restart when runtime config is identical.
	limit := 2
	if e := f.app.ChangeUserDevices(ctx, f.User.ID, record.Version, "", "", &limit); e != nil {
		t.Fatal(e)
	}
	for _, node := range f.Nodes {
		before, e := f.app.Store.NodeControl(ctx, node.ID)
		if e != nil {
			t.Fatal(e)
		}
		if e := f.app.ReconcileNodeCredentials(ctx, node.ID); e != nil {
			t.Fatal(e)
		}
		after, _ := f.app.Store.NodeControl(ctx, node.ID)
		if after.Desired != before.Desired || after.DeploymentID != before.DeploymentID {
			t.Fatal("unchanged config restarted node")
		}
	}
	record, _ = f.app.Store.UserDevices(ctx, f.User.ID)
	if e := f.app.ChangeUserDevices(ctx, f.User.ID, record.Version, "", device.ID, nil); e != nil {
		t.Fatal(e)
	}
	if _, e := f.app.DeviceLinks(ctx, f.User.ID, device.ID); !errors.Is(e, store.ErrNotFound) {
		t.Fatal("revoked export allowed", e)
	}
	for i := range f.Nodes {
		command := poll(i)
		if strings.Contains(string(command.Config), device.UUID) || strings.Contains(string(command.Config), f.User.UUID) {
			t.Fatal("revocation retained a credential")
		}
		ack(i, command)
	}
	if nodes, e := f.app.Store.ReadyDeviceNodes(ctx); e != nil || len(nodes) != 2 {
		t.Fatal("revocation not confirmed", e)
	}
	if e := f.app.Store.CheckPackageCoverage(ctx, 1024, 0); !errors.Is(e, store.ErrUsageUnsupported) {
		t.Fatal("remote finite quota gate removed")
	}
}

func TestRemoteDeviceRevokePersistsWithoutHelperAndBackupIsInert(t *testing.T) {
	f := newCascadeUIFixture(t)
	ctx := context.Background()
	for _, node := range f.Nodes {
		if e := f.app.Store.Heartbeat(ctx, node.Secret, store.NodeHeartbeat{ProtocolVersion: 1, DeviceProtocol: 1, XrayState: "active"}); e != nil {
			t.Fatal(e)
		}
	}
	if e := f.app.ChangeUserDevices(ctx, f.User.ID, 0, "TV", "", nil); e != nil {
		t.Fatal(e)
	}
	record, _ := f.app.Store.UserDevices(ctx, f.User.ID)
	device := record.Devices[0]
	if e := f.app.Store.Heartbeat(ctx, f.Nodes[0].Secret, store.NodeHeartbeat{ProtocolVersion: 1, XrayState: "unknown"}); e != nil {
		t.Fatal(e)
	}
	if e := f.app.ChangeUserDevices(ctx, f.User.ID, record.Version, "", device.ID, nil); e != nil {
		t.Fatal("offline revoke lost", e)
	}
	record, _ = f.app.Store.UserDevices(ctx, f.User.ID)
	if record.Devices[0].RevokedAt == 0 {
		t.Fatal("revocation not persisted")
	}
	if e := f.app.ChangeUserDevices(ctx, f.User.ID, record.Version, "Another TV", "", nil); !errors.Is(e, store.ErrUsageUnsupported) {
		t.Fatal("new device on unsupported node accepted", e)
	}
	snapshot, e := f.app.ExportState(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if e := snapshot.Validate(); e != nil {
		t.Fatal("remote device export invalid", e)
	}
	if e := f.app.RestoreState(ctx, snapshot); e != nil {
		t.Fatal("inert restore failed", e)
	}
	if _, e := f.app.DeviceLinks(ctx, f.User.ID, device.ID); !errors.Is(e, store.ErrNotFound) {
		t.Fatal("restore reactivated revoked device", e)
	}
	for _, node := range f.Nodes {
		state, e := f.app.Store.NodeControl(ctx, node.ID)
		if e != nil || state.Published || state.Applied != 0 {
			t.Fatal("restore trusted old runtime", e)
		}
	}
}
