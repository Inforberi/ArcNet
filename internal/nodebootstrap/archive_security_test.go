package nodebootstrap

import (
	"github.com/arcvpn/arcvpn/internal/updater"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadReleaseRejectsUnsafeArchiveWithoutReadingIt(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "key.hex")
	if err := os.WriteFile(keyPath, []byte(strings.Repeat("1", 64)+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(dir, "release.zip")
	if err := os.WriteFile(archive, []byte("invalid archive"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "symlink.zip")
	if err := os.Symlink(archive, link); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRelease(link, keyPath); err == nil {
		t.Fatal("accepted symlink archive")
	}
	if _, err := LoadRelease(archive, keyPath); err == nil {
		t.Fatal("accepted unsigned archive")
	}
	if err := os.Truncate(archive, updater.MaxArchive+1); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRelease(archive, keyPath); err == nil {
		t.Fatal("accepted oversized archive")
	}
}

func TestInstallRollbackDeletesOnlyAccountsItCreated(t *testing.T) {
	script := installBody("/tmp/arcvpn-stage4-test", "af1b8595-c790-4a4a-83ad-8aee0ef4c305", "0.6.3-stage4", []byte("agent"), []byte("xray"))
	for _, required := range []string{
		"created_node=0", "created_xray=0", "created_node=1", "created_xray=1",
		`if [ "$created_node" -eq 1 ]; then userdel arcvpn-node`,
		`if [ "$created_xray" -eq 1 ]; then userdel arcvpn-xray`,
	} {
		if !strings.Contains(script, required) {
			t.Fatalf("unsafe account rollback, missing %q", required)
		}
	}
}
