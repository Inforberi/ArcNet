package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadProtectedProbeFile(t *testing.T) {
	dir := t.TempDir()
	secret := filepath.Join(dir, "link.txt")
	if err := os.WriteFile(secret, []byte("vless://private-data"), 0600); err != nil {
		t.Fatal(err)
	}
	if got, err := readProtectedFile(secret, 100); err != nil || string(got) != "vless://private-data" {
		t.Fatalf("private file read: %q, %v", got, err)
	}
	if err := os.Chmod(secret, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := readProtectedFile(secret, 100); err == nil {
		t.Fatal("world-readable secret was accepted")
	}
	if err := os.Chmod(secret, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readProtectedFile(secret, 3); err == nil {
		t.Fatal("oversized secret was accepted")
	}
	link := filepath.Join(dir, "symlink.txt")
	if err := os.Symlink(secret, link); err != nil {
		t.Fatal(err)
	}
	if _, err := readProtectedFile(link, 100); err == nil {
		t.Fatal("symlink to secret was accepted")
	}
	if _, err := readProtectedFile(dir, 100); err == nil {
		t.Fatal("directory was accepted")
	}
}

func TestNodeProbeArgumentValidationDoesNotRunXray(t *testing.T) {
	for _, args := range [][]string{
		{},
		{"--link-file", "/does-not-exist", "extra"},
		{"--link-file", "/does-not-exist", "--report-token-stdin", "--report-token-file", "/does-not-exist"},
	} {
		if err := runNodeProbe(args); err == nil {
			t.Fatalf("accepted invalid args: %v", args)
		}
	}
	file := filepath.Join(t.TempDir(), "bad-link")
	if err := os.WriteFile(file, []byte("https://invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := runNodeProbe([]string{"--link-file", file, "--xray", "/does-not-exist"}); err == nil || !strings.Contains(err.Error(), "VLESS") {
		t.Fatalf("untrusted link was not rejected before spawning Xray: %v", err)
	}
}
