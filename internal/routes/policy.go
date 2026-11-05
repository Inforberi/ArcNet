// Package routes implements the Stage 5 routing control-plane model.
// It deliberately cannot apply Xray configuration or publish subscriptions:
// a draft is not a verified data-plane route.
package routes

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

const LocalNode = "local"
const MaxPolicies = 64
const MaxBackups = 4

var nodeIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,80}$`)

// Policy represents intent. No field here can execute remote commands.
// EntryNode and exit IDs are stable node identities, never host addresses.
type Policy struct {
	ID                 string   `json:"id"`
	Name               string   `json:"name"`
	Mode               string   `json:"mode"` // direct | cascade
	EntryNode          string   `json:"entry_node"`
	PrimaryExit        string   `json:"primary_exit,omitempty"`
	BackupExits        []string `json:"backup_exits"`
	FailureMode        string   `json:"failure_mode"` // closed | open
	FailuresToSwitch   int      `json:"failures_to_switch"`
	SuccessesToRecover int      `json:"successes_to_recover"`
	CooldownSeconds    int      `json:"cooldown_seconds"`
	Version            int64    `json:"version"`
	UpdatedAt          int64    `json:"updated_at"`
}

func DefaultPolicy() Policy {
	return Policy{Mode: "direct", EntryNode: LocalNode, FailureMode: "closed", FailuresToSwitch: 3, SuccessesToRecover: 3, CooldownSeconds: 60, BackupExits: []string{}}
}

func Validate(p Policy) error {
	if n := len([]rune(strings.TrimSpace(p.Name))); n < 1 || n > 80 || strings.TrimSpace(p.Name) != p.Name || strings.ContainsAny(p.Name, "\x00\r\n") {
		return errors.New("invalid route name (1–80 characters)")
	}
	if p.Mode != "direct" && p.Mode != "cascade" {
		return errors.New("mode must be direct or cascade")
	}
	if !nodeIDPattern.MatchString(p.EntryNode) {
		return errors.New("invalid entry node ID")
	}
	if p.FailureMode != "closed" && p.FailureMode != "open" {
		return errors.New("failure_mode must be closed or open")
	}
	if p.FailuresToSwitch < 1 || p.FailuresToSwitch > 20 || p.SuccessesToRecover < 1 || p.SuccessesToRecover > 20 {
		return errors.New("hysteresis thresholds must be between 1 and 20")
	}
	if p.CooldownSeconds < 0 || p.CooldownSeconds > 86400 {
		return errors.New("cooldown must be within 0–86400 seconds")
	}
	if p.Mode == "direct" {
		if p.PrimaryExit != "" || len(p.BackupExits) != 0 {
			return errors.New("direct routes cannot specify exits")
		}
		return nil
	}
	if !nodeIDPattern.MatchString(p.PrimaryExit) || p.PrimaryExit == p.EntryNode {
		return errors.New("cascade must use a distinct valid primary exit")
	}
	if len(p.BackupExits) > MaxBackups {
		return errors.New("too many backup exits")
	}
	seen := map[string]bool{p.EntryNode: true, p.PrimaryExit: true}
	for _, id := range p.BackupExits {
		if !nodeIDPattern.MatchString(id) || seen[id] {
			return fmt.Errorf("invalid or duplicated backup exit %q", id)
		}
		seen[id] = true
	}
	return nil
}

// Edges includes backup links: even a dormant backup may become an active hop.
type Edge struct {
	From     string `json:"from"`
	To       string `json:"to"`
	PolicyID string `json:"policy_id"`
	Backup   bool   `json:"backup"`
}

func Edges(policies []Policy) []Edge {
	out := []Edge{}
	for _, p := range policies {
		if p.Mode != "cascade" {
			continue
		}
		out = append(out, Edge{From: p.EntryNode, To: p.PrimaryExit, PolicyID: p.ID})
		for _, b := range p.BackupExits {
			out = append(out, Edge{From: p.EntryNode, To: b, PolicyID: p.ID, Backup: true})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].From != out[j].From {
			return out[i].From < out[j].From
		}
		if out[i].To != out[j].To {
			return out[i].To < out[j].To
		}
		return out[i].PolicyID < out[j].PolicyID
	})
	return out
}

// ValidateGraph prevents cascades that could form forwarding loops across policies.
// It does not claim that the real Xray runtime has been configured.
func ValidateGraph(policies []Policy) error {
	if len(policies) > MaxPolicies {
		return fmt.Errorf("route limit %d exceeded", MaxPolicies)
	}
	ids := map[string]bool{}
	for _, p := range policies {
		if err := Validate(p); err != nil {
			return fmt.Errorf("policy %q: %w", p.ID, err)
		}
		if p.ID == "" || !nodeIDPattern.MatchString(p.ID) || ids[p.ID] {
			return fmt.Errorf("duplicate or invalid policy ID %q", p.ID)
		}
		ids[p.ID] = true
	}
	adjacency := map[string][]string{}
	for _, edge := range Edges(policies) {
		adjacency[edge.From] = append(adjacency[edge.From], edge.To)
	}
	colors := map[string]uint8{}
	var walk func(string) error
	walk = func(node string) error {
		if colors[node] == 1 {
			return fmt.Errorf("cycle detected through node %q", node)
		}
		if colors[node] == 2 {
			return nil
		}
		colors[node] = 1
		for _, next := range adjacency[node] {
			if err := walk(next); err != nil {
				return err
			}
		}
		colors[node] = 2
		return nil
	}
	keys := make([]string, 0, len(adjacency))
	for k := range adjacency {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if err := walk(k); err != nil {
			return err
		}
	}
	return nil
}
