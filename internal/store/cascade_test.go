package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/arcvpn/arcvpn/internal/db"
	"github.com/arcvpn/arcvpn/internal/routes"
)

type cascadeFixture struct {
	s                    *Store
	ctx                  context.Context
	a, b, sa, sb         string
	input                CascadePreparation
	baselineA, baselineB []byte
}

func newCascadeFixture(t *testing.T) *cascadeFixture {
	t.Helper()
	s, closeDB := createControlTestStore(t)
	t.Cleanup(closeDB)
	f := &cascadeFixture{s: s, ctx: context.Background(), baselineA: []byte(`{"baseline":"entry"}`), baselineB: []byte(`{"baseline":"exit"}`)}
	f.a, f.sa = createControlTestNode(t, s)
	f.b, f.sb = createControlTestNode(t, s)
	if e := s.UpdateNodeEndpoint(f.ctx, f.b, "1.1.1.1", 443); e != nil {
		t.Fatal(e)
	}
	for node, secret := range map[string]string{f.a: f.sa, f.b: f.sb} {
		if e := s.Heartbeat(f.ctx, secret, NodeHeartbeat{ProtocolVersion: 1, CascadeProtocol: 1, XrayState: "active"}); e != nil {
			t.Fatal(e)
		}
		config := f.baselineA
		if node == f.b {
			config = f.baselineB
		}
		if _, e := s.QueueNodeDeployment(f.ctx, node, config); e != nil {
			t.Fatal(e)
		}
		cmd, e := s.PendingNodeCommand(f.ctx, node, secret)
		if e != nil || cmd == nil {
			t.Fatal("baseline command", e)
		}
		f.ack(t, cmd, node, secret, "applied")
	}
	p := routes.DefaultPolicy()
	p.Name = "A to B"
	p.Mode = "cascade"
	p.EntryNode = f.a
	p.PrimaryExit = f.b
	saved, e := s.SaveRoute(f.ctx, p, 0)
	if e != nil {
		t.Fatal(e)
	}
	roster, e := s.CascadeRoster(f.ctx)
	if e != nil {
		t.Fatal(e)
	}
	f.input = CascadePreparation{RouteID: saved.ID, ExpectedVersion: saved.Version, RosterDigest: roster, EntryConfig: []byte(`{"secret":"entry-machine-secret"}`), ExitConfig: []byte(`{"secret":"exit-machine-private"}`), ChannelSecret: []byte(`{"key":"independent-channel-private"}`)}
	return f
}
func (f *cascadeFixture) ack(t *testing.T, cmd *NodeCommand, node, secret, result string) {
	t.Helper()
	if e := f.s.AcknowledgeNodeOperation(f.ctx, node, secret, cmd.OperationID, cmd.Revision, cmd.SHA256, result); e != nil {
		t.Fatal("ack", e)
	}
}
func (f *cascadeFixture) prepare(t *testing.T) CascadeDeployment {
	t.Helper()
	d, e := f.s.PrepareCascade(f.ctx, f.input)
	if e != nil {
		t.Fatal(e)
	}
	return d
}
func (f *cascadeFixture) start(t *testing.T, d CascadeDeployment) CascadeDeployment {
	t.Helper()
	d, e := f.s.StartCascade(f.ctx, d.ID, d.Version)
	if e != nil {
		t.Fatal(e)
	}
	return d
}
func (f *cascadeFixture) command(t *testing.T, node, secret string) *NodeCommand {
	t.Helper()
	c, e := f.s.PendingNodeCommand(f.ctx, node, secret)
	if e != nil || c == nil {
		t.Fatal("command missing", e)
	}
	return c
}
func (f *cascadeFixture) state(t *testing.T, id, state string) CascadeDeployment {
	t.Helper()
	d, e := f.s.CascadeDeployment(f.ctx, id)
	if e != nil || d.State != state {
		t.Fatalf("expected %s, got %s (%v)", state, d.State, e)
	}
	return d
}
func TestCascadeJournalOrderedEncryptedAndRestart(t *testing.T) {
	f := newCascadeFixture(t)
	d := f.prepare(t)
	if c, e := f.s.PendingNodeCommand(f.ctx, f.a, f.sa); e != nil || c != nil {
		t.Fatal("prepared entry changed runtime")
	}
	if e := f.s.DB.Read(f.ctx, func(tx *db.Tx) error {
		rows, e := tx.Query("SELECT secret_ciphertext FROM cascade_deployments WHERE id=?", d.ID)
		if e != nil {
			return e
		}
		if strings.Contains(rows[0]["secret_ciphertext"], "independent-channel-private") {
			t.Fatal("plaintext secret")
		}
		plain, e := f.s.openNodeConfig(rows[0]["secret_ciphertext"], "cascade-secret:"+d.ID)
		if e != nil || string(plain) != string(f.input.ChannelSecret) {
			t.Fatal("secret persistence", e)
		}
		if _, e := f.s.openNodeConfig(rows[0]["secret_ciphertext"], "cascade-secret:another-id"); e == nil {
			t.Fatal("AAD not bound to deployment")
		}
		steps, e := tx.Query("SELECT config_ciphertext FROM cascade_steps WHERE deployment_id=?", d.ID)
		if e != nil {
			return e
		}
		for _, step := range steps {
			if strings.Contains(step["config_ciphertext"], "machine") {
				t.Fatal("plaintext config")
			}
		}
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	d = f.start(t, d)
	if c, e := f.s.PendingNodeCommand(f.ctx, f.a, f.sa); e != nil || c != nil {
		t.Fatal("entry was queued before exit ACK")
	}
	exit := f.command(t, f.b, f.sb)
	if exit.Kind != "apply_cascade_xray_v1" {
		t.Fatal("wrong typed kind")
	}
	// A controller restart retains command identity and encrypted config.
	path := f.s.DB.Path()
	if e := f.s.DB.Close(); e != nil {
		t.Fatal(e)
	}
	reopened, e := db.Open(path)
	if e != nil {
		t.Fatal(e)
	}
	f.s.DB = reopened
	t.Cleanup(func() { _ = reopened.Close() })
	if e := f.s.Migrate(f.ctx); e != nil {
		t.Fatal(e)
	}
	replay := f.command(t, f.b, f.sb)
	if replay.OperationID != exit.OperationID || replay.Revision != exit.Revision || string(replay.Config) != string(exit.Config) {
		t.Fatal("restart changed command identity")
	}
	if e := f.s.AcknowledgeNodeOperation(f.ctx, f.b, "wrong-secret", exit.OperationID, exit.Revision, exit.SHA256, "applied"); !errors.Is(e, ErrNodeUnauthorized) {
		t.Fatal("unauthenticated ACK", e)
	}
	if e := f.s.AcknowledgeNodeOperation(f.ctx, f.b, f.sb, exit.OperationID, exit.Revision, strings.Repeat("0", 64), "applied"); !errors.Is(e, ErrStaleNodeOperation) {
		t.Fatal("wrong digest ACK", e)
	}
	f.ack(t, exit, f.b, f.sb, "applied")
	f.ack(t, exit, f.b, f.sb, "applied")
	f.state(t, d.ID, "entry_queued")
	entry := f.command(t, f.a, f.sa)
	f.ack(t, entry, f.a, f.sa, "applied")
	f.state(t, d.ID, "awaiting_probe")
	for _, node := range []string{f.a, f.b} {
		c, e := f.s.NodeControl(f.ctx, node)
		if e != nil || c.Published || c.Probe == "pass" {
			t.Fatal("ACK published or proved VPN")
		}
	}
	blob, _ := json.Marshal(d)
	if strings.Contains(string(blob), "secret") || strings.Contains(string(blob), "ciphertext") {
		t.Fatal("public DTO leaked secret")
	}
}
func TestCascadeJournalCompensatesEntryBeforeExit(t *testing.T) {
	f := newCascadeFixture(t)
	d := f.start(t, f.prepare(t))
	exit := f.command(t, f.b, f.sb)
	f.ack(t, exit, f.b, f.sb, "applied")
	entry := f.command(t, f.a, f.sa)
	f.ack(t, entry, f.a, f.sa, "failed")
	f.state(t, d.ID, "rollback_entry")
	rollback := f.command(t, f.a, f.sa)
	if rollback.Kind != "apply_xray_v1" || rollback.Revision <= entry.Revision || string(rollback.Config) != string(f.baselineA) {
		t.Fatal("invalid entry compensation")
	}
	if cmd, e := f.s.PendingNodeCommand(f.ctx, f.b, f.sb); e != nil || cmd != nil {
		t.Fatal("exit removed before entry restore")
	}
	f.ack(t, rollback, f.a, f.sa, "applied")
	f.state(t, d.ID, "rollback_exit")
	restored := f.command(t, f.b, f.sb)
	if restored.Revision <= exit.Revision || string(restored.Config) != string(f.baselineB) {
		t.Fatal("invalid exit compensation")
	}
	f.ack(t, restored, f.b, f.sb, "applied")
	f.state(t, d.ID, "rolled_back")
	if _, e := f.s.QueueNodeDeployment(f.ctx, f.a, f.baselineA); e != nil {
		t.Fatal("node lease not released", e)
	}
}
func TestCascadeJournalLostACKRetainsLeasesAndRequiresRecovery(t *testing.T) {
	f := newCascadeFixture(t)
	d := f.start(t, f.prepare(t))
	exit := f.command(t, f.b, f.sb)
	for i := 1; i < maxNodeDeliveryAttempts; i++ {
		cmd := f.command(t, f.b, f.sb)
		if cmd.OperationID != exit.OperationID {
			t.Fatal("lost ACK changed identity")
		}
	}
	if cmd, e := f.s.PendingNodeCommand(f.ctx, f.b, f.sb); e != nil || cmd != nil {
		t.Fatal("unbounded delivery")
	}
	d = f.state(t, d.ID, "attention")
	if _, e := f.s.QueueNodeDeployment(f.ctx, f.a, f.baselineA); !errors.Is(e, ErrCascadeBusy) {
		t.Fatal("unknown operation released lease", e)
	}
	if e := f.s.RollbackCascade(f.ctx, d.ID, d.Version); e != nil {
		t.Fatal(e)
	}
	entry := f.command(t, f.a, f.sa)
	f.ack(t, entry, f.a, f.sa, "applied")
	restored := f.command(t, f.b, f.sb)
	if e := f.s.AcknowledgeNodeOperation(f.ctx, f.b, f.sb, exit.OperationID, exit.Revision, exit.SHA256, "applied"); !errors.Is(e, ErrStaleNodeOperation) {
		t.Fatal("late ACK accepted", e)
	}
	f.ack(t, restored, f.b, f.sb, "applied")
	f.state(t, d.ID, "rolled_back")
}
func TestCascadeJournalCASGuardsAndSafeCleanup(t *testing.T) {
	f := newCascadeFixture(t)
	d := f.prepare(t)
	if _, e := f.s.QueueNodeDeployment(f.ctx, f.a, f.baselineA); !errors.Is(e, ErrCascadeBusy) {
		t.Fatal("direct apply bypassed reservation", e)
	}
	if e := f.s.DeleteUser(f.ctx, 123); !errors.Is(e, ErrCascadeBusy) {
		t.Fatal("user change bypassed reservation", e)
	}
	if e := f.s.UpdateNodeEndpoint(f.ctx, f.a, "8.8.8.8", 443); !errors.Is(e, ErrCascadeBusy) {
		t.Fatal("endpoint change bypassed reservation", e)
	}
	if e := f.s.DeleteRoute(f.ctx, d.RouteID, d.RouteVersion); !errors.Is(e, ErrCascadeBusy) {
		t.Fatal("route deletion bypassed journal", e)
	}
	if e := f.s.ReplaceState(f.ctx, nil, nil); !errors.Is(e, ErrCascadeBusy) {
		t.Fatal("restore bypassed journal", e)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, e := f.s.StartCascade(f.ctx, d.ID, d.Version); results <- e }()
	}
	wg.Wait()
	close(results)
	successes := 0
	conflicts := 0
	for e := range results {
		if e == nil {
			successes++
		} else if errors.Is(e, ErrConflict) {
			conflicts++
		} else {
			t.Fatal(e)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatal("CAS allowed two starts")
	}
	if e := f.s.DeleteCascade(f.ctx, d.ID, d.Version); !errors.Is(e, ErrConflict) {
		t.Fatal("active journal deleted", e)
	}
}
func TestCascadeJournalRejectsMissingCapabilityAndRosterDrift(t *testing.T) {
	f := newCascadeFixture(t)
	if e := f.s.Heartbeat(f.ctx, f.sa, NodeHeartbeat{ProtocolVersion: 1, XrayState: "active"}); e != nil {
		t.Fatal(e)
	}
	if _, e := f.s.PrepareCascade(f.ctx, f.input); !errors.Is(e, ErrCascadeNotReady) {
		t.Fatal("old helper accepted", e)
	}
	if e := f.s.Heartbeat(f.ctx, f.sa, NodeHeartbeat{ProtocolVersion: 1, CascadeProtocol: 1, XrayState: "active"}); e != nil {
		t.Fatal(e)
	}
	f.input.RosterDigest = strings.Repeat("0", 64)
	if _, e := f.s.PrepareCascade(f.ctx, f.input); !errors.Is(e, ErrConflict) {
		t.Fatal("stale roster accepted", e)
	}
}

func TestCascadeJournalExitFailureAndFailedCompensation(t *testing.T) {
	f := newCascadeFixture(t)
	d := f.start(t, f.prepare(t))
	exit := f.command(t, f.b, f.sb)
	f.ack(t, exit, f.b, f.sb, "failed")
	f.state(t, d.ID, "rollback_exit")
	if cmd, e := f.s.PendingNodeCommand(f.ctx, f.a, f.sa); e != nil || cmd != nil {
		t.Fatal("entry changed after exit failure")
	}
	restore := f.command(t, f.b, f.sb)
	f.ack(t, restore, f.b, f.sb, "failed")
	d = f.state(t, d.ID, "attention")
	if e := f.s.DeleteCascade(f.ctx, d.ID, d.Version); !errors.Is(e, ErrConflict) {
		t.Fatal("failed compensation deleted", e)
	}
	if _, e := f.s.QueueNodeDeployment(f.ctx, f.b, f.baselineB); !errors.Is(e, ErrCascadeBusy) {
		t.Fatal("failed compensation released lease", e)
	}
}
func TestCascadeJournalCapabilityWithdrawnAfterStart(t *testing.T) {
	f := newCascadeFixture(t)
	d := f.start(t, f.prepare(t))
	if e := f.s.Heartbeat(f.ctx, f.sb, NodeHeartbeat{ProtocolVersion: 1, XrayState: "active"}); e != nil {
		t.Fatal(e)
	}
	if cmd, e := f.s.PendingNodeCommand(f.ctx, f.b, f.sb); e != nil || cmd != nil {
		t.Fatal("command dispatched to downgraded helper", e)
	}
	f.state(t, d.ID, "attention")
}
