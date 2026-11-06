package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"

	"github.com/arcvpn/arcvpn/internal/store"
)

type fakeSystemd struct {
	states  map[string]string
	fail    string
	calls   []string
	changes []string
}

func (s *fakeSystemd) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	if name != "/usr/bin/systemctl" || len(args) < 2 {
		return nil, errors.New("non-allowlisted command")
	}
	action, unit := args[0], args[len(args)-1]
	known := false
	for _, item := range units {
		if item.unit == unit {
			known = true
		}
	}
	if !known {
		return nil, errors.New("unknown unit")
	}
	s.calls = append(s.calls, action+" "+unit)
	if action == "show" {
		state, exists := s.states[unit]
		if !exists {
			return []byte("LoadState=not-found\nActiveState=inactive\n"), nil
		}
		if state == "unknown" {
			return nil, errors.New("D-Bus unavailable")
		}
		return []byte(fmt.Sprintf("ActiveState=%s\nLoadState=loaded\n", state)), nil
	}
	s.changes = append(s.changes, action+" "+unit)
	if action+" "+unit == s.fail {
		return nil, errors.New("command failed with private output")
	}
	if action == "stop" {
		s.states[unit] = "inactive"
	} else {
		s.states[unit] = "active"
	}
	return nil, nil
}

func managerFixture(t *testing.T) (Manager, *fakeSystemd) {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("root coordination fixture in disposable Linux")
	}
	systemd := &fakeSystemd{states: map[string]string{}}
	for _, item := range units {
		systemd.states[item.unit] = "inactive"
	}
	plan := store.RuntimePlan{Xray: true, WireGuard: true}
	m := Manager{Runner: systemd, Guard: Guard{Dir: filepath.Join(t.TempDir(), "runtime")}, Plan: func(context.Context) (store.RuntimePlan, error) { return plan, nil }}
	m.Prepare = func(context.Context) error {
		if systemd.states[units[0].unit] != "inactive" {
			return errors.New("panel writer still running")
		}
		systemd.changes = append(systemd.changes, "prepare")
		return nil
	}
	m.Apply = func(context.Context) error {
		if err := m.Guard.Check(); err != nil {
			return err
		}
		if systemd.states[units[0].unit] != "inactive" || systemd.states[units[5].unit] != "active" {
			return errors.New("unsafe apply order")
		}
		systemd.changes = append(systemd.changes, "apply")
		for _, item := range units[1:5] {
			if desired(plan, false)[item.name] {
				systemd.states[item.unit] = "active"
			} else {
				systemd.states[item.unit] = "inactive"
			}
		}
		return nil
	}
	return m, systemd
}

func TestFullLifecycleOrderIdempotenceAndDisabledProfiles(t *testing.T) {
	m, s := managerFixture(t)
	ctx := context.Background()
	for _, item := range units {
		s.states[item.unit] = "active"
	}
	report, err := m.Execute(ctx, "stop", "")
	if err != nil || !report.Paused {
		t.Fatal(report, err)
	}
	want := []string{"stop arcvpn-panel.service", "prepare", "stop arcvpn-control.socket", "stop arcvpn-control.service", "stop arcvpn-amneziawg.service", "stop arcvpn-wireguard.service", "stop arcvpn-hysteria.service", "stop arcvpn-xray.service"}
	if !slices.Equal(s.changes, want) {
		t.Fatal("stop order", s.changes)
	}
	s.changes = nil
	if _, err := m.Execute(ctx, "stop", ""); err != nil || len(s.changes) != 0 {
		t.Fatal("stop not idempotent", s.changes, err)
	}
	if report, err = m.Execute(ctx, "start", ""); err != nil || report.Paused {
		t.Fatal(report, err)
	}
	if s.states[units[2].unit] != "inactive" || s.states[units[4].unit] != "inactive" {
		t.Fatal("disabled/legacy profile started")
	}
	if slices.Index(s.changes, "apply") < 0 || slices.Index(s.changes, "start arcvpn-panel.service") < slices.Index(s.changes, "apply") {
		t.Fatal("panel opened before policy apply", s.changes)
	}
	s.changes = nil
	if _, err := m.Execute(ctx, "start", ""); err != nil || len(s.changes) != 0 {
		t.Fatal("start not idempotent", s.changes, err)
	}
	if _, err := m.Execute(ctx, "restart", ""); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(s.changes, "apply") {
		t.Fatal("restart did not reconcile")
	}
}

func TestStopFailureDoesNotRaceLiveWriter(t *testing.T) {
	m, s := managerFixture(t)
	ctx := context.Background()
	s.states[units[0].unit] = "active"
	s.states[units[1].unit] = "active"
	s.fail = "stop arcvpn-panel.service"
	report, err := m.Execute(ctx, "stop", "")
	if err == nil || !report.Paused || len(report.Components) == 0 {
		t.Fatal(report, err)
	}
	if len(s.changes) != 1 || s.states[units[1].unit] != "active" {
		t.Fatal("engine touched while writer running", s.changes)
	}
}

func TestPartialStopContinuesAndReportsActualFailure(t *testing.T) {
	m, s := managerFixture(t)
	for _, item := range units {
		s.states[item.unit] = "active"
	}
	s.fail = "stop arcvpn-wireguard.service"
	report, err := m.Execute(context.Background(), "stop", "")
	if err == nil || s.states[units[1].unit] != "inactive" || s.states[units[3].unit] != "active" {
		t.Fatal(report, err)
	}
	if !strings.Contains(err.Error(), "wireguard") {
		t.Fatal("failure omitted", err)
	}
}

func TestStartFailureClosesPartialRuntimeAndPreservesPause(t *testing.T) {
	m, s := managerFixture(t)
	m.Apply = func(context.Context) error { s.states[units[3].unit] = "active"; return errors.New("apply failed") }
	report, err := m.Execute(context.Background(), "start", "")
	if err == nil || !report.Paused {
		t.Fatal(report, err)
	}
	for _, item := range units {
		if s.states[item.unit] == "active" {
			t.Fatal("partial listener left running", item.unit)
		}
	}
	if !errors.Is(m.Guard.Check(), ErrPaused) {
		t.Fatal("failed start lost marker")
	}
}

func TestCleanupFailureIsNotReportedAsStopped(t *testing.T) {
	m, s := managerFixture(t)
	m.Apply = func(context.Context) error {
		s.states[units[3].unit] = "active"
		s.fail = "stop arcvpn-wireguard.service"
		return errors.New("apply failed")
	}
	report, err := m.Execute(context.Background(), "start", "")
	if err == nil || !strings.Contains(err.Error(), "cleanup incomplete") {
		t.Fatal(report, err)
	}
	if report.Components[3].Actual != "active" || report.Components[3].Error == "" {
		t.Fatal("unverified cleanup claimed success", report)
	}
}

func TestLifecycleLockRejectsConcurrentCommandWithoutMutation(t *testing.T) {
	m, s := managerFixture(t)
	unlock, err := m.Guard.Lock()
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	if _, err := m.Execute(context.Background(), "stop", ""); !errors.Is(err, ErrBusy) || len(s.changes) > 0 {
		t.Fatal(s.changes, err)
	}
	if err := m.Guard.Check(); err != nil {
		t.Fatal("busy command changed pause", err)
	}
}

func TestComponentRestartAndInvalidTargets(t *testing.T) {
	m, s := managerFixture(t)
	ctx := context.Background()
	for _, index := range []int{0, 1, 3, 5, 6} {
		s.states[units[index].unit] = "active"
	}
	if _, err := m.Execute(ctx, "restart", "xray"); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(s.changes, "stop arcvpn-wireguard.service") || slices.Contains(s.changes, "restart arcvpn-wireguard.service") || !slices.Contains(s.changes, "restart arcvpn-xray.service") {
		t.Fatal("component restart disrupted another engine", s.changes)
	}
	s.changes = nil
	for _, target := range []string{"sshd", "--root=/tmp", "hysteria"} {
		if _, err := m.Execute(ctx, "restart", target); err == nil {
			t.Fatal("unsafe/disabled target accepted", target)
		}
	}
	if len(s.changes) > 0 {
		t.Fatal("invalid target mutated system", s.changes)
	}
}

func TestUnavailablePolicyAndUnknownStateDoNotStartRuntime(t *testing.T) {
	m, s := managerFixture(t)
	m.Plan = func(context.Context) (store.RuntimePlan, error) {
		return store.RuntimePlan{}, errors.New("database unavailable")
	}
	if _, err := m.Execute(context.Background(), "start", ""); err == nil || len(s.changes) > 0 {
		t.Fatal(s.changes, err)
	}
	s.states[units[0].unit] = "unknown"
	if _, err := m.Execute(context.Background(), "stop", ""); err == nil || len(s.changes) > 0 {
		t.Fatal("unknown treated as stopped", s.changes, err)
	}
}

func TestGuardRejectsLinksWritableDirectoryAndSpecialFiles(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root fixture")
	}
	dir := t.TempDir()
	g := Guard{Dir: filepath.Join(dir, "operations")}
	unlock, err := g.Lock()
	if err != nil {
		t.Fatal(err)
	}
	unlock()
	if err := os.Symlink(filepath.Join(dir, "other"), filepath.Join(g.Dir, "paused")); err != nil {
		t.Fatal(err)
	}
	if g.Check() == nil || g.Resume() == nil {
		t.Fatal("unsafe marker accepted")
	}
	if err := os.Remove(filepath.Join(g.Dir, "paused")); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(g.Dir, 0777); err != nil {
		t.Fatal(err)
	}
	if _, err := g.Lock(); err == nil {
		t.Fatal("writable directory accepted")
	}
	if err := os.Chmod(g.Dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(g.Dir, "lock")); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "private")
	if err := os.WriteFile(target, []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(g.Dir, "lock")); err != nil {
		t.Fatal(err)
	}
	if _, err := g.Lock(); err == nil {
		t.Fatal("lock symlink accepted")
	}
	if content, _ := os.ReadFile(target); string(content) != "private" {
		t.Fatal("lock target changed")
	}
	if err := os.Remove(filepath.Join(g.Dir, "lock")); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(g.Dir, "lock"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := g.Lock(); err == nil {
		t.Fatal("FIFO accepted as lock")
	}
}

func TestGuardMakesMarkerDirectorySearchableWithPrivateRootUmask(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root fixture")
	}
	g := Guard{Dir: filepath.Join(t.TempDir(), "private")}
	if err := os.Mkdir(g.Dir, 0700); err != nil {
		t.Fatal(err)
	}
	unlock, err := g.Lock()
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	info, err := os.Stat(g.Dir)
	if err != nil || info.Mode().Perm() != 0755 {
		t.Fatal("service user cannot inspect marker", err)
	}
}
