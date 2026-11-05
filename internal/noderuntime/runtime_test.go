package noderuntime_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/arcvpn/arcvpn/internal/noderuntime"
	"github.com/arcvpn/arcvpn/internal/store"
	"github.com/arcvpn/arcvpn/internal/xray"
)

type fakeRunner struct {
	mu          sync.Mutex
	failRestart bool
	calls       []string
}

func (f *fakeRunner) Run(ctx context.Context, name string, args ...string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cmd := name + " " + strings.Join(args, " ")
	f.calls = append(f.calls, cmd)
	if f.failRestart && strings.Contains(cmd, "systemctl restart") {
		f.failRestart = false
		return errors.New("simulated Xray failure")
	}
	return nil
}
func validRequest(t *testing.T, revision int64) noderuntime.ApplyRequest {
	t.Helper()
	r := store.Reality{Address: "9.9.9.9", Port: 443, Target: "www.example.com", SNI: "www.example.com", Private: strings.Repeat("A", 43), Public: strings.Repeat("B", 43), ShortID: "0123456789abcdef", Enabled: true}
	users := []store.User{{Name: "Client", UUID: "c9cdf017-50e4-45ef-9a20-7a9b3cf7bc8b", Enabled: true}}
	b, e := xray.Generate(r, users)
	if e != nil {
		t.Fatal(e)
	}
	var compact bytes.Buffer
	if e := json.Compact(&compact, b); e != nil {
		t.Fatal(e)
	}
	b = compact.Bytes()
	sum := sha256.Sum256(b)
	return noderuntime.ApplyRequest{ProtocolVersion: 1, Kind: "apply_xray_v1", OperationID: "11111111-2222-4333-8444-555555555555", Revision: revision, SHA256: hex.EncodeToString(sum[:]), Config: b}
}
func TestValidateStrictOperationsAndConfig(t *testing.T) {
	req := validRequest(t, 1)
	if e := noderuntime.ValidateRequest(req); e != nil {
		t.Fatal(e)
	}
	malformed := req
	malformed.Kind = "shell"
	if e := noderuntime.ValidateRequest(malformed); e == nil {
		t.Fatal("shell command accepted")
	}
	malformed = req
	malformed.SHA256 = strings.Repeat("0", 64)
	if e := noderuntime.ValidateRequest(malformed); e == nil {
		t.Fatal("bad digest accepted")
	}
	var config map[string]any
	if e := json.Unmarshal(req.Config, &config); e != nil {
		t.Fatal(e)
	}
	config["api"] = map[string]string{"tag": "admin"}
	b, _ := json.Marshal(config)
	if e := noderuntime.ValidateConfig(b); e == nil {
		t.Fatal("Xray privileged API field accepted")
	}
	config["api"] = nil
	delete(config, "api")
	config["outbounds"] = []any{map[string]string{"protocol": "freedom", "tag": "direct", "settings": "danger"}}
	b, _ = json.Marshal(config)
	if e := noderuntime.ValidateConfig(b); e == nil {
		t.Fatal("arbitrary outbound settings accepted")
	}
}
func TestApplySuccessDuplicateAndRollback(t *testing.T) {
	dir := t.TempDir()
	conf := filepath.Join(dir, "xray.json")
	receipt := filepath.Join(dir, "last-apply.json")
	runner := &fakeRunner{}
	installer := noderuntime.Installer{ConfigPath: conf, ReceiptPath: receipt, XrayBinary: "/fake/xray", Runner: runner}
	req := validRequest(t, 1)
	first, e := installer.Apply(context.Background(), req)
	if e != nil || first.Status != "applied" {
		t.Fatalf("first apply: %+v %v", first, e)
	}
	content, e := os.ReadFile(conf)
	if e != nil || string(content) != string(req.Config) {
		t.Fatal("new config was not written")
	}
	dup, e := installer.Apply(context.Background(), req)
	if e != nil || dup.Status != "applied" {
		t.Fatalf("duplicate apply: %+v %v", dup, e)
	}
	conflicts := req
	conflicts.OperationID = "99999999-2222-4333-8444-555555555555"
	if _, e = installer.Apply(context.Background(), conflicts); e == nil {
		t.Fatal("conflicting replay accepted")
	}
	next := validRequest(t, 2)
	next.OperationID = "bbbbbbbb-2222-4333-8444-555555555555"
	// Modify a harmless generated field to force new config bytes and digest.
	var root map[string]any
	_ = json.Unmarshal(next.Config, &root)
	root["log"] = map[string]string{"loglevel": "error"}
	next.Config, _ = json.Marshal(root)
	sum := sha256.Sum256(next.Config)
	next.SHA256 = hex.EncodeToString(sum[:])
	runner.failRestart = true
	reverted, e := installer.Apply(context.Background(), next)
	if e != nil || reverted.Status != "rolled_back" {
		t.Fatalf("expected rollback: %+v %v", reverted, e)
	}
	after, _ := os.ReadFile(conf)
	if string(after) != string(req.Config) {
		t.Fatal("rollback lost last-good config")
	}
	// After failed apply the old operation remains the last acknowledged revision.
	stale := validRequest(t, 0)
	if _, e = installer.Apply(context.Background(), stale); e == nil {
		t.Fatal("downgrade accepted")
	}
}
func TestUnixHelperRequestRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "helper.sock")
	server, e := net.Listen("unix", path)
	if e != nil {
		t.Fatal(e)
	}
	defer server.Close()
	dir := t.TempDir()
	installer := noderuntime.Installer{XrayBinary: "/fake/xray", ConfigPath: filepath.Join(dir, "xray.json"), ReceiptPath: filepath.Join(dir, "last-apply.json"), Runner: &fakeRunner{}}
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, e := server.Accept()
		if e != nil {
			return
		}
		noderuntime.ServeConn(context.Background(), conn, installer)
	}()
	result, e := noderuntime.CallHelper(context.Background(), path, validRequest(t, 1))
	if e != nil || result.Status != "applied" {
		t.Fatalf("Unix helper response: %+v %v", result, e)
	}
	<-done
}

// Losing an ACK must never convert a corrupted file or a stopped service
// into a successful state report merely because a receipt still exists.
func TestDuplicateOperationDoesNotTrustStaleReceipt(t *testing.T) {
	dir := t.TempDir()
	conf := filepath.Join(dir, "xray.json")
	receipt := filepath.Join(dir, "last-apply.json")
	runner := &fakeRunner{}
	installer := noderuntime.Installer{ConfigPath: conf, ReceiptPath: receipt, XrayBinary: "/fake/xray", Runner: runner}
	req := validRequest(t, 1)
	if _, err := installer.Apply(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(conf, []byte(`{"malicious":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	if result, err := installer.Apply(context.Background(), req); err == nil || result.Status == "applied" {
		t.Fatalf("stale receipt trusted with corrupt config: %+v %v", result, err)
	}
	if err := os.WriteFile(conf, req.Config, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(conf); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(receipt, conf); err != nil {
		t.Fatal(err)
	}
	if result, err := installer.Apply(context.Background(), req); err == nil || result.Status == "applied" {
		t.Fatalf("stale receipt trusted with symlink config: %+v %v", result, err)
	}
}

type inactiveRunner struct{ fakeRunner }

func (r *inactiveRunner) Run(ctx context.Context, name string, args ...string) error {
	if name == "/usr/bin/systemctl" && len(args) >= 1 && args[0] == "is-active" {
		return errors.New("service failed")
	}
	return r.fakeRunner.Run(ctx, name, args...)
}
func TestDuplicateOperationRejectsStoppedService(t *testing.T) {
	dir := t.TempDir()
	conf := filepath.Join(dir, "xray.json")
	receipt := filepath.Join(dir, "last-apply.json")
	good := &fakeRunner{}
	installer := noderuntime.Installer{ConfigPath: conf, ReceiptPath: receipt, XrayBinary: "/fake/xray", Runner: good}
	req := validRequest(t, 1)
	if _, err := installer.Apply(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	installer.Runner = &inactiveRunner{}
	if result, err := installer.Apply(context.Background(), req); err == nil || result.Status == "applied" {
		t.Fatalf("inactive service treated as healthy: %+v %v", result, err)
	}
}

func TestSymlinkReceiptCannotBeTrusted(t *testing.T) {
	dir := t.TempDir()
	conf := filepath.Join(dir, "xray.json")
	receipt := filepath.Join(dir, "last-apply.json")
	actual := filepath.Join(dir, "original.json")
	if err := os.WriteFile(actual, []byte(`{"id":"forged"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(actual, receipt); err != nil {
		t.Fatal(err)
	}
	installer := noderuntime.Installer{ConfigPath: conf, ReceiptPath: receipt, XrayBinary: "/fake/xray", Runner: &fakeRunner{}}
	if result, err := installer.Apply(context.Background(), validRequest(t, 1)); err == nil || result.Status == "applied" {
		t.Fatalf("symlink receipt accepted: %+v %v", result, err)
	}
}

func TestCorruptReceiptBlocksDowngradeAndReplay(t *testing.T) {
	dir := t.TempDir()
	receipt := filepath.Join(dir, "last-apply.json")
	if err := os.WriteFile(receipt, []byte("not json"), 0600); err != nil {
		t.Fatal(err)
	}
	installer := noderuntime.Installer{ConfigPath: filepath.Join(dir, "xray.json"), ReceiptPath: receipt, XrayBinary: "/fake/xray", Runner: &fakeRunner{}}
	if result, err := installer.Apply(context.Background(), validRequest(t, 1)); err == nil || result.Status == "applied" {
		t.Fatalf("corrupt receipt allowed apply: %+v %v", result, err)
	}
}
