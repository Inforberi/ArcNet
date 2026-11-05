package routes

import "testing"

func cascade(id, entry, exit string, backups ...string) Policy {
	p := DefaultPolicy()
	p.ID = id
	p.Name = id
	p.Mode = "cascade"
	p.EntryNode = entry
	p.PrimaryExit = exit
	p.BackupExits = backups
	return p
}
func TestValidateGraph(t *testing.T) {
	for _, tc := range []struct {
		name  string
		items []Policy
		valid bool
	}{
		{"simple", []Policy{cascade("ab", "a", "b")}, true},
		{"reverse cycle", []Policy{cascade("ab", "a", "b"), cascade("ba", "b", "a")}, false},
		{"backup cycle", []Policy{cascade("ab", "a", "b", "c"), cascade("ca", "c", "a")}, false},
		{"independent directions", []Policy{cascade("ab", "a", "b"), cascade("cd", "c", "d")}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := ValidateGraph(tc.items)
			if (e == nil) != tc.valid {
				t.Fatalf("got %v", e)
			}
		})
	}
}
func TestRejectUnsafeRoutes(t *testing.T) {
	p := cascade("ab", "a", "a")
	if Validate(p) == nil {
		t.Fatal("self cascade allowed")
	}
	p = cascade("ab", "a", "b", "a")
	if Validate(p) == nil {
		t.Fatal("entry as backup allowed")
	}
	p = cascade("ab", "a", "b", "b")
	if Validate(p) == nil {
		t.Fatal("duplicate primary allowed")
	}
	p = DefaultPolicy()
	p.Name = "one"
	p.PrimaryExit = "b"
	if Validate(p) == nil {
		t.Fatal("direct with exit allowed")
	}
}
func TestSimulatorHysteresisAndFailClosed(t *testing.T) {
	p := cascade("r", "a", "b", "c")
	p.FailuresToSwitch = 2
	p.SuccessesToRecover = 2
	p.CooldownSeconds = 30
	samples := []Sample{
		{At: 100, Health: map[string]string{"a": "healthy", "b": "healthy", "c": "healthy"}},
		{At: 101, Health: map[string]string{"a": "healthy", "b": "unhealthy", "c": "healthy"}},
		{At: 130, Health: map[string]string{"a": "healthy", "b": "unhealthy", "c": "healthy"}},
		{At: 131, Health: map[string]string{"a": "healthy", "b": "healthy", "c": "healthy"}},
		{At: 162, Health: map[string]string{"a": "healthy", "b": "healthy", "c": "healthy"}},
	}
	results, err := Simulate(p, samples)
	if err != nil {
		t.Fatal(err)
	}
	expected := []string{"cascade", "blocked", "cascade", "cascade", "cascade"}
	for i, r := range results {
		if r.EffectivePath != expected[i] {
			t.Fatalf("step %d: %+v", i, r)
		}
	}
	if results[1].ActiveExit != "b" || results[2].ActiveExit != "c" || results[4].ActiveExit != "b" {
		t.Fatal("switch or recover failed", results)
	}
}
func TestSimulatorUnknownNeverHealthy(t *testing.T) {
	p := cascade("r", "a", "b", "c")
	p.FailuresToSwitch = 1
	output, e := Simulate(p, []Sample{{At: 1, Health: map[string]string{"b": "unknown", "c": "healthy"}}})
	if e != nil {
		t.Fatal(e)
	}
	if output[0].EffectivePath != "blocked" {
		t.Fatalf("unknown primary must not trigger failover: %+v", output[0])
	}
	p.FailureMode = "open"
	output, e = Simulate(p, []Sample{{At: 1, Health: map[string]string{"a": "healthy", "b": "unhealthy"}}})
	if e != nil || output[0].EffectivePath != "fail_open" {
		t.Fatal(output, e)
	}
}

func TestUnavailableEntryBlocksCascadeAndFailOpen(t *testing.T) {
	p := cascade("r", "a", "b", "c")
	p.FailureMode = "open"
	got, e := Simulate(p, []Sample{{At: 100, Health: map[string]string{"a": "unhealthy", "b": "healthy", "c": "healthy"}}})
	if e != nil || got[0].EffectivePath != "blocked" {
		t.Fatal("unreachable entry was treated as a working route", got, e)
	}
}
