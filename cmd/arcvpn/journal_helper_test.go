package main

import "testing"

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
