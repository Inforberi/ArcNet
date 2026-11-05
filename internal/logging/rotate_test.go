package logging

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestRotateBounds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	r, err := New(path, 50)
	if err != nil {
		t.Fatal(err)
	}
	if r.BudgetMB() != 50 {
		t.Fatal("wrong budget")
	}
	if err = r.SetBudgetMB(20); err == nil {
		t.Fatal("invalid limit accepted")
	}
	// exercise multiple rotations with bounded total retained bytes
	entry := bytes.Repeat([]byte("x"), 1<<20)
	for i := 0; i < 60; i++ {
		if _, err := r.Write(entry); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	var total int64
	files, _ := filepath.Glob(path + "*")
	for _, f := range files {
		stat, err := os.Stat(f)
		if err != nil {
			t.Fatal(err)
		}
		total += stat.Size()
	}
	if total > 50<<20 {
		t.Fatalf("log budget exceeded: %d bytes", total)
	}
	if len(files) != 5 {
		t.Fatalf("expected five rotating log segments, got %d", len(files))
	}
}
