package nodeagent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestReadConfigRejectsSymlinkAndOversizedCredential(t *testing.T) {
	directory := t.TempDir()
	config := Configuration{Controller: "https://controller.example.com", NodeID: "node-1", Secret: strings.Repeat("s", 48)}
	blob, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	realFile := filepath.Join(directory, "real.json")
	if err := os.WriteFile(realFile, blob, 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(directory, "node.json")
	if err := os.Symlink(realFile, link); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadConfig(link); err == nil {
		t.Fatal("accepted a symlink to credentials")
	}
	if got, err := ReadConfig(realFile); err != nil || got.NodeID != config.NodeID {
		t.Fatalf("valid credentials refused: %+v %v", got, err)
	}
	if err := os.WriteFile(realFile, []byte(strings.Repeat("x", maxCredentialFileSize+1)), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadConfig(realFile); err == nil {
		t.Fatal("accepted oversized credential payload")
	}
	if err := os.Chmod(realFile, 0640); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadConfig(realFile); err == nil {
		t.Fatal("accepted group-readable credentials")
	}
}

func TestReadConfigRejectsFIFOWithoutBlocking(t *testing.T) {
	// An attacker-controlled path must never be able to hang agent startup
	// while an open() blocks waiting for a FIFO writer. O_NONBLOCK plus a
	// regular-file check enforces this without launching another goroutine.
	directory := t.TempDir()
	fifo := filepath.Join(directory, "node.pipe")
	if err := syscall.Mkfifo(fifo, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadConfig(fifo); err == nil {
		t.Fatal("accepted non-regular credentials")
	}
}
