// Package firewall contains a deliberately limited UFW adapter. It never enables
// or disables UFW and never modifies a rule without the exact ArcVPN marker.
package firewall

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/arcvpn/arcvpn/internal/ops"
)

const marker = "ArcVPN managed VPN"

var numbered = regexp.MustCompile(`^\[\s*(\d+)\]\s+(\d+)/(tcp|udp)(?:\s+\(v6\))?\s+ALLOW\s+IN\s+.*#\s+ArcVPN managed VPN\s*$`)

type entry struct {
	number int
	rule   ops.FirewallRule
}

func parse(text string) ([]entry, error) {
	var entries []entry
	for _, line := range strings.Split(text, "\n") {
		if !strings.Contains(line, marker) {
			continue
		}
		m := numbered.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			return nil, errors.New("cannot safely parse ArcVPN UFW rule")
		}
		id, _ := strconv.Atoi(m[1])
		port, _ := strconv.Atoi(m[2])
		if id < 1 || port < 1 || port > 65535 || port == 22 || port == 80 {
			return nil, errors.New("invalid ArcVPN rule")
		}
		entries = append(entries, entry{id, ops.FirewallRule{Port: port, Transport: m[3]}})
	}
	return entries, nil
}

type Runner interface {
	Run(context.Context, ...string) (string, error)
}
type CLI struct{}

func (CLI) Run(ctx context.Context, args ...string) (string, error) {
	c := exec.CommandContext(ctx, "/usr/sbin/ufw", args...)
	c.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LC_ALL=C", "LANG=C"}
	b, err := c.CombinedOutput()
	if len(b) > 128*1024 {
		return "", errors.New("UFW output too large")
	}
	if err != nil {
		return "", fmt.Errorf("UFW command failed: %w", err)
	}
	return string(b), nil
}

type Service struct{ Runner Runner }

func (s Service) runner() Runner {
	if s.Runner != nil {
		return s.Runner
	}
	return CLI{}
}
func (s Service) Status(ctx context.Context) (ops.FirewallStatus, error) {
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	out, err := s.runner().Run(ctx, "status", "numbered")
	if err != nil {
		return ops.FirewallStatus{}, err
	}
	first := strings.SplitN(out, "\n", 2)[0]
	active := first == "Status: active"
	if !active && first != "Status: inactive" {
		return ops.FirewallStatus{}, errors.New("unknown UFW state")
	}
	parsed, err := parse(out)
	if err != nil {
		return ops.FirewallStatus{}, err
	}
	rules := make([]ops.FirewallRule, 0, len(parsed))
	seen := map[string]bool{}
	for _, p := range parsed {
		k := fmt.Sprintf("%s/%d", p.rule.Transport, p.rule.Port)
		if !seen[k] {
			rules = append(rules, p.rule)
			seen[k] = true
		}
	}
	sort.Slice(rules, func(i, j int) bool {
		if rules[i].Port == rules[j].Port {
			return rules[i].Transport < rules[j].Transport
		}
		return rules[i].Port < rules[j].Port
	})
	return ops.FirewallStatus{Active: active, Rules: rules}, nil
}
func (s Service) Apply(ctx context.Context, action string, rule ops.FirewallRule) (ops.FirewallStatus, error) {
	if action != "allow" && action != "remove" {
		return ops.FirewallStatus{}, errors.New("operation denied")
	}
	if rule.Port <= 0 || rule.Port > 65535 || rule.Port == 22 || rule.Port == 80 || (rule.Transport != "tcp" && rule.Transport != "udp") {
		return ops.FirewallStatus{}, errors.New("port or transport denied")
	}
	before, err := s.Status(ctx)
	if err != nil {
		return ops.FirewallStatus{}, err
	}
	if !before.Active {
		return ops.FirewallStatus{}, errors.New("UFW inactive: refusing to change global firewall state")
	}
	known := false
	for _, r := range before.Rules {
		if r == rule {
			known = true
			break
		}
	}
	if action == "allow" {
		if known {
			return before, nil
		}
		// Do not take ownership of an existing rule created by the administrator.
		current, err := s.runner().Run(ctx, "status", "numbered")
		if err != nil {
			return ops.FirewallStatus{}, err
		}
		if strings.Contains(current, fmt.Sprintf("%d/%s", rule.Port, rule.Transport)+" ") {
			return ops.FirewallStatus{}, errors.New("port already has an unowned UFW rule")
		}
		if _, err = s.runner().Run(ctx, "allow", fmt.Sprintf("%d/%s", rule.Port, rule.Transport), "comment", marker); err != nil {
			return ops.FirewallStatus{}, err
		}
	} else {
		if !known {
			return before, nil
		}
		output, err := s.runner().Run(ctx, "status", "numbered")
		if err != nil {
			return ops.FirewallStatus{}, err
		}
		parsed, err := parse(output)
		if err != nil {
			return ops.FirewallStatus{}, err
		}
		// UFW numbered rules shift after deletion: delete from highest index to lowest.
		var matched []int
		for _, p := range parsed {
			if p.rule == rule {
				matched = append(matched, p.number)
			}
		}
		sort.Sort(sort.Reverse(sort.IntSlice(matched)))
		if len(matched) == 0 {
			return ops.FirewallStatus{}, errors.New("managed rule disappeared")
		}
		for _, index := range matched {
			if _, err = s.runner().Run(ctx, "--force", "delete", strconv.Itoa(index)); err != nil {
				return ops.FirewallStatus{}, err
			}
		}
	}
	after, err := s.Status(ctx)
	if err != nil {
		return ops.FirewallStatus{}, err
	}
	exists := false
	for _, r := range after.Rules {
		if r == rule {
			exists = true
		}
	}
	if (action == "allow") != exists {
		return ops.FirewallStatus{}, errors.New("UFW did not reach the requested state")
	}
	return after, nil
}
