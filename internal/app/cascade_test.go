package app

import (
	"context"
	"testing"

	"github.com/arcvpn/arcvpn/internal/noderuntime"
	"github.com/arcvpn/arcvpn/internal/routes"
	"github.com/arcvpn/arcvpn/internal/store"
	"github.com/arcvpn/arcvpn/internal/xray"
)

func TestCascadePreparationGeneratesDeliverableConfigs(t *testing.T) {
	a := firstProtocolApp(t)
	ctx := context.Background()
	nodes := []string{}
	secrets := []string{}
	for _, ip := range []string{"9.9.9.9", "1.1.1.1"} {
		token, e := a.Store.CreateEnrollmentForNode(ctx, "fixture", ip, 443)
		if e != nil {
			t.Fatal(e)
		}
		node, secret, e := a.Store.RegisterNode(ctx, token)
		if e != nil {
			t.Fatal(e)
		}
		if e := a.Store.Heartbeat(ctx, secret, store.NodeHeartbeat{ProtocolVersion: 1, CascadeProtocol: 1, XrayState: "active"}); e != nil {
			t.Fatal(e)
		}
		key, e := xray.NewCascadeChannel(14443)
		if e != nil {
			t.Fatal(e)
		}
		profile := store.Reality{Address: ip, Port: 443, Target: "www.example.com", SNI: "www.example.com", Private: key.Private, Public: key.Public, ShortID: key.ShortID, Enabled: true}
		if e := a.Store.PutNodeRealityIfAbsent(ctx, node.ID, profile); e != nil {
			t.Fatal(e)
		}
		baseline, e := xray.Generate(profile, nil)
		if e != nil {
			t.Fatal(e)
		}
		if _, e := a.Store.QueueNodeDeployment(ctx, node.ID, baseline); e != nil {
			t.Fatal(e)
		}
		command, e := a.Store.PendingNodeCommand(ctx, node.ID, secret)
		if e != nil || command == nil {
			t.Fatal(e)
		}
		if e := a.Store.AcknowledgeNodeOperation(ctx, node.ID, secret, command.OperationID, command.Revision, command.SHA256, "applied"); e != nil {
			t.Fatal(e)
		}
		nodes = append(nodes, node.ID)
		secrets = append(secrets, secret)
	}
	p := routes.DefaultPolicy()
	p.Name = "fixture cascade"
	p.Mode = "cascade"
	p.EntryNode = nodes[0]
	p.PrimaryExit = nodes[1]
	p, e := a.Store.SaveRoute(ctx, p, 0)
	if e != nil {
		t.Fatal(e)
	}
	deployment, e := a.PrepareCascade(ctx, p.ID, p.Version, 15443)
	if e != nil {
		t.Fatal(e)
	}
	deployment, e = a.Store.StartCascade(ctx, deployment.ID, deployment.Version)
	if e != nil {
		t.Fatal(e)
	}
	for _, idx := range []int{1, 0} {
		command, e := a.Store.PendingNodeCommand(ctx, nodes[idx], secrets[idx])
		if e != nil || command == nil {
			t.Fatal(e)
		}
		request := noderuntime.ApplyRequest{ProtocolVersion: command.ProtocolVersion, Kind: command.Kind, OperationID: command.OperationID, Revision: command.Revision, SHA256: command.SHA256, Config: command.Config}
		if e := noderuntime.ValidateRequest(request); e != nil {
			t.Fatal("prepared command cannot pass helper validation", e)
		}
		if e := a.Store.AcknowledgeNodeOperation(ctx, nodes[idx], secrets[idx], command.OperationID, command.Revision, command.SHA256, "applied"); e != nil {
			t.Fatal(e)
		}
	}
	deployment, e = a.Store.CascadeDeployment(ctx, deployment.ID)
	if e != nil || deployment.State != "awaiting_probe" {
		t.Fatal("not waiting for independent probe", e)
	}
}
