package noderuntime_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/arcvpn/arcvpn/internal/noderuntime"
	"github.com/arcvpn/arcvpn/internal/store"
	"github.com/arcvpn/arcvpn/internal/xray"
)

func TestCascadeApplyReceiptAndRollback(t *testing.T) {
	entryKey, e := xray.NewCascadeChannel(11443)
	if e != nil {
		t.Fatal(e)
	}
	exitKey, e := xray.NewCascadeChannel(12443)
	if e != nil {
		t.Fatal(e)
	}
	channel, e := xray.NewCascadeChannel(13443)
	if e != nil {
		t.Fatal(e)
	}
	profile := func(ip string, c xray.CascadeChannel) store.Reality {
		return store.Reality{Address: ip, Port: c.Port, Private: c.Private, Public: c.Public, ShortID: c.ShortID, Target: "www.example.com", SNI: "www.example.com", Enabled: true}
	}
	a, b, e := xray.GenerateCascade(profile("9.9.9.9", entryKey), profile("1.1.1.1", exitKey), nil, nil, channel)
	if e != nil {
		t.Fatal(e)
	}
	for name, config := range map[string][]byte{"entry": a, "exit": b} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			runner := &fakeRunner{}
			installer := noderuntime.Installer{ConfigPath: filepath.Join(dir, "xray.json"), ReceiptPath: filepath.Join(dir, "receipt.json"), XrayBinary: "/fake/xray", Runner: runner}
			previous := validRequest(t, 1)
			if result, e := installer.Apply(context.Background(), previous); e != nil || result.Status != "applied" {
				t.Fatal("baseline apply", e)
			}
			request := previous
			request.Kind = "apply_cascade_xray_v1"
			request.OperationID = "22222222-2222-4222-8222-222222222222"
			request.Revision = 2
			request.Config = config
			sum := sha256.Sum256(config)
			request.SHA256 = hex.EncodeToString(sum[:])
			runner.failRestart = true
			if result, e := installer.Apply(context.Background(), request); e != nil || result.Status != "rolled_back" {
				t.Fatal("rollback", result, e)
			}
			bytes, e := os.ReadFile(installer.ConfigPath)
			if e != nil || string(bytes) != string(previous.Config) {
				t.Fatal("last-good config lost")
			}
			if result, e := installer.Apply(context.Background(), request); e != nil || result.Status != "applied" {
				t.Fatal("apply", result, e)
			}
			calls := len(runner.calls)
			if result, e := installer.Apply(context.Background(), request); e != nil || result.Status != "applied" {
				t.Fatal("lost-ACK replay", result, e)
			}
			for _, call := range runner.calls[calls:] {
				if call == "/usr/bin/systemctl restart "+noderuntime.Service {
					t.Fatal("duplicate operation restarted service")
				}
			}
			legacy := request
			legacy.Kind = "apply_xray_v1"
			if e := noderuntime.ValidateRequest(legacy); e == nil {
				t.Fatal("legacy operation accepted new config")
			}
			request.SHA256 = hex.EncodeToString(make([]byte, 32))
			if e := noderuntime.ValidateRequest(request); e == nil {
				t.Fatal("bad payload digest accepted")
			}
		})
	}
}
