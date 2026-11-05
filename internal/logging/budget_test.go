package logging

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBudgetPrunesOldRotationsImmediately(t *testing.T) {
	p := filepath.Join(t.TempDir(), "panel.log")
	if err := os.WriteFile(p, make([]byte, 3<<20), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p+".1", make([]byte, 50<<20), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p+".2", make([]byte, 50<<20), 0600); err != nil {
		t.Fatal(err)
	}
	r, err := New(p, 50)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if used := r.UsedBytes(); used > 50<<20 {
		t.Fatalf("budget exceeded on startup: %d", used)
	}
	if err = r.SetBudgetMB(100); err != nil {
		t.Fatal(err)
	}
	if n, err := r.Write([]byte("test")); err != nil || n != 4 {
		t.Fatal(n, err)
	}
}
func TestSymlinkLogPathRejected(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	link := filepath.Join(dir, "panel.log")
	if err := os.WriteFile(target, []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := New(link, 500); err == nil {
		t.Fatal("symlink accepted")
	}
}
