package ops

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

// FirewallRule describes a rule owned by ArcVPN, not arbitrary host firewall rules.
type FirewallRule struct {
	Port      int    `json:"port"`
	Transport string `json:"transport"`
}
type FirewallStatus struct {
	Active bool           `json:"active"`
	Rules  []FirewallRule `json:"rules"`
}

func Firewall(ctx context.Context, socket string, action string, rule FirewallRule) (FirewallStatus, error) {
	if socket == "" {
		return FirewallStatus{}, ErrUnavailable
	}
	command := "firewall status\n"
	if action != "status" {
		if (action != "allow" && action != "remove") || (rule.Transport != "tcp" && rule.Transport != "udp") || rule.Port < 1 || rule.Port > 65535 || rule.Port == 22 || rule.Port == 80 {
			return FirewallStatus{}, errors.New("invalid firewall operation")
		}
		command = fmt.Sprintf("firewall %s %s %d\n", action, rule.Transport, rule.Port)
	}
	d := net.Dialer{Timeout: 2 * time.Second}
	conn, err := d.DialContext(ctx, "unix", socket)
	if err != nil {
		return FirewallStatus{}, ErrUnavailable
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(15 * time.Second))
	if _, err = io.WriteString(conn, command); err != nil {
		return FirewallStatus{}, ErrUnavailable
	}
	b, err := io.ReadAll(io.LimitReader(conn, 8193))
	if err != nil || len(b) > 8192 {
		return FirewallStatus{}, ErrUnavailable
	}
	var result FirewallStatus
	if json.Unmarshal(b, &result) != nil || result.Rules == nil {
		return FirewallStatus{}, ErrUnavailable
	}
	for _, r := range result.Rules {
		if (r.Transport != "tcp" && r.Transport != "udp") || r.Port < 1 || r.Port > 65535 {
			return FirewallStatus{}, ErrUnavailable
		}
	}
	return result, nil
}

func ParseFirewallRule(protocol string, port string) (FirewallRule, error) {
	n, err := strconv.Atoi(strings.TrimSpace(port))
	if err != nil || n <= 0 || n > 65535 {
		return FirewallRule{}, errors.New("invalid port")
	}
	if protocol != "tcp" && protocol != "udp" {
		return FirewallRule{}, errors.New("invalid transport")
	}
	return FirewallRule{Port: n, Transport: protocol}, nil
}
