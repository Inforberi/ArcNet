package backupfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveAndPrune(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 5; i++ {
		p, err := Save(dir, []byte("encrypted"), 2)
		if err != nil {
			t.Fatal(err)
		}
		st, err := os.Stat(p)
		if err != nil || st.Mode().Perm() != 0600 {
			t.Fatal(st, err)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 2 {
		t.Fatalf("expected 2: %d %v", len(entries), err)
	}
	stray := filepath.Join(dir, "other.txt")
	if err := os.WriteFile(stray, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Prune(dir, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stray); err != nil {
		t.Fatal("deleted unrelated file")
	}
}
func TestPrivatePasswordFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "password")
	if err := os.WriteFile(p, []byte("another-long-secret\n"), 0600); err != nil {
		t.Fatal(err)
	}
	pass, err := PasswordFile(p)
	if err != nil || pass != "another-long-secret" {
		t.Fatal(pass, err)
	}
	if err := os.Chmod(p, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := PasswordFile(p); err == nil {
		t.Fatal("publicly readable passphrase accepted")
	}
	if err := os.Symlink(p, filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	if _, err := PasswordFile(filepath.Join(dir, "link")); err == nil {
		t.Fatal("symlink accepted")
	}
	if strings.Contains(p, "../") {
		t.Fatal("invalid path")
	}
}
