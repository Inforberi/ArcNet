package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRuntimePlanParserRejectsIncompleteOrExecutableInput(t *testing.T) {
	valid := `{"xray":true,"hysteria":false,"wireguard":true,"legacy_amnezia":false}`
	plan, err := parseRuntimePlan([]byte(valid))
	if err != nil || !plan.Xray || !plan.WireGuard || plan.Hysteria {
		t.Fatal(plan, err)
	}
	for _, data := range []string{`{}`, `null`, valid + `{}`, `{"xray":"true"}`, `{"xray":true,"hysteria":false,"wireguard":true,"legacy_amnezia":false,"unit":"sshd"}`} {
		if _, err := parseRuntimePlan([]byte(data)); err == nil {
			t.Fatal("unsafe policy accepted", data)
		}
	}
}

func TestLifecycleExecCancellationKillsWholeChildGroup(t *testing.T) {
	output := filepath.Join(t.TempDir(), "orphan-write")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := (lifecycleExec{}).Run(ctx, "/bin/sh", "-c", `(sleep .3; printf leaked > "$1") & wait`, "fixture", output)
	if err == nil {
		t.Fatal("canceled command succeeded")
	}
	time.Sleep(400 * time.Millisecond)
	if _, err := os.Stat(output); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("orphan child continued writing after cancellation", err)
	}
}

func TestRuntimeCommandsRejectRootBeforeCreatingDatabase(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root denial fixture")
	}
	previous := os.Args
	defer func() { os.Args = previous }()
	dir := filepath.Join(t.TempDir(), "must-not-be-created")
	t.Setenv("ARCVPN_DATA_DIR", dir)
	for _, command := range []string{"runtime-plan", "runtime-prepare", "runtime-apply"} {
		os.Args = []string{"arcvpn", command}
		if err := run(); err == nil {
			t.Fatal("root database access allowed", command)
		}
		if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("runtime command initialized state", err)
		}
	}
}

func TestLifecycleRejectsInvalidArgumentsBeforeExecutingCommands(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root CLI fixture")
	}
	for _, args := range [][]string{{"start", "xray"}, {"restart", "sshd"}, {"status", "--unit=sshd"}, {"logs", "--root=/tmp"}, {"unknown"}} {
		err := runLifecycle(args)
		var coded interface{ ExitCode() int }
		if !errors.As(err, &coded) || coded.ExitCode() != 2 {
			t.Fatal(args, err)
		}
	}
}
