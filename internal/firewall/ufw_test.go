package firewall

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/arcvpn/arcvpn/internal/ops"
)

type scripted struct {
	output string
	calls  [][]string
	fail   string
}

func (s *scripted) Run(_ context.Context, args ...string) (string, error) {
	s.calls = append(s.calls, append([]string(nil), args...))
	cmd := strings.Join(args, " ")
	if strings.Contains(cmd, s.fail) && s.fail != "" {
		return "", errors.New("forced failure")
	}
	if strings.HasPrefix(cmd, "allow ") {
		s.output = "Status: active\n[ 1] 443/tcp  ALLOW IN Anywhere # ArcVPN managed VPN\n[ 2] 443/tcp (v6) ALLOW IN Anywhere (v6) # ArcVPN managed VPN\n"
	}
	if cmd == "--force delete 2" {
		s.output = "Status: active\n[ 1] 443/tcp ALLOW IN Anywhere # ArcVPN managed VPN\n"
	}
	if cmd == "--force delete 1" {
		s.output = "Status: active\n"
	}
	return s.output, nil
}
func TestManagedUFWLifecycle(t *testing.T) {
	runner := &scripted{output: "Status: active\n[ 1] 22/tcp ALLOW IN Anywhere\n"}
	svc := Service{Runner: runner}
	status, e := svc.Apply(context.Background(), "allow", ops.FirewallRule{Port: 443, Transport: "tcp"})
	if e != nil || len(status.Rules) != 1 {
		t.Fatal(e, status)
	}
	status, e = svc.Apply(context.Background(), "remove", ops.FirewallRule{Port: 443, Transport: "tcp"})
	if e != nil || len(status.Rules) != 0 {
		t.Fatal(e, status)
	}
	for _, args := range runner.calls {
		if strings.Join(args, " ") == "--force delete 3" {
			t.Fatal("deleted unowned rule")
		}
	}
}
func TestFirewallFailClosed(t *testing.T) {
	for _, v := range []string{"Status: active\n[ 7] 22/tcp ALLOW IN Anywhere # ArcVPN managed VPN\n", "Status: active\nArcVPN managed VPN\n", "Status: active\n[ 3] 443/tcp DENY IN Anywhere # ArcVPN managed VPN\n"} {
		_, err := (Service{Runner: &scripted{output: v}}).Status(context.Background())
		if err == nil {
			t.Fatal("accepted malformed rule", v)
		}
	}
	s := Service{Runner: &scripted{output: "Status: inactive\n"}}
	if _, err := s.Apply(context.Background(), "allow", ops.FirewallRule{Port: 443, Transport: "tcp"}); err == nil {
		t.Fatal("modified inactive firewall")
	}
	for _, port := range []int{22, 80, -1, 65536} {
		if _, err := s.Apply(context.Background(), "remove", ops.FirewallRule{Port: port, Transport: "tcp"}); err == nil {
			t.Fatalf("accepted protected port %d", port)
		}
	}
}
