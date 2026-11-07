package main

import "testing"

func TestFirewallReadDispatch(t *testing.T) {
	if !firewallReadCommand("firewall status\n") {
		t.Fatal("missing read-only firewall dispatch")
	}
	for _, line := range []string{"firewall allow tcp 3443\n", "firewall remove udp 8444\n", "firewall status\nextra", "firewall status", "firewall enable\n"} {
		if firewallReadCommand(line) {
			t.Fatal("unexpected firewall operation", line)
		}
	}
}

func TestParseJournalBudget(t *testing.T) {
	for _, tc := range []struct {
		line  string
		valid bool
	}{
		{"journal-budget 50\n", true}, {"journal-budget 1024\n", true},
		{"journal-budget 49\n", false}, {"journal-budget 1025\n", false},
		{"journal-budget -1\n", false}, {"journal-budget 100\nextra", false},
		{"journal-budget 100.0\n", false}, {"journal-budget 100; rm -rf /\n", false},
	} {
		_, err := parseJournalBudget(tc.line)
		if (err == nil) != tc.valid {
			t.Fatalf("%q: %v", tc.line, err)
		}
	}
}

func TestUpdateHelperDispatchAndUnitState(t *testing.T) {
	for _, action := range []string{"status", "verify", "apply"} {
		if got, ok := updateCommand("update " + action + "\n"); !ok || got != action {
			t.Fatal("missing update dispatch", action)
		}
	}
	for _, line := range []string{"update apply other\n", "update shell\n", "update apply", "update apply\nextra", "update status \n"} {
		if _, ok := updateCommand(line); ok {
			t.Fatal("unexpected helper command", line)
		}
	}
	state, err := parseUpdateUnit([]byte("LoadState=loaded\nActiveState=activating\nResult=success\nExecMainStartTimestampMonotonic=1234\n"))
	if err != nil || !state.Busy || state.StartedAt != 1234 {
		t.Fatal("unit state", err)
	}
	if _, err = parseUpdateUnit([]byte("LoadState=not-found\nActiveState=inactive\nResult=success\n")); err == nil {
		t.Fatal("unknown unit accepted")
	}
}
