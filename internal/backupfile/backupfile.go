// Package backupfile persists encrypted ArcVPN snapshots atomically for scheduled backups.
package backupfile

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// PasswordFile reads a locally provisioned 0600 regular file, never an API value.
func PasswordFile(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 1025 {
		return "", errors.New("backup passphrase file must be regular, private (0600), and at most 1025 bytes")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	value := strings.TrimSuffix(string(b), "\n")
	if len(value) < 12 || len(value) > 1024 {
		return "", errors.New("backup passphrase requires 12–1024 bytes")
	}
	return value, nil
}

// Save creates a private, atomic archive and rotates only matching files in
// the dedicated output directory. No arbitrary path is taken from the browser.
func Save(directory string, blob []byte, keep int) (string, error) {
	if keep < 1 || keep > 30 || len(blob) == 0 || len(blob) > 10<<20 {
		return "", errors.New("invalid archive or retention")
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		return "", err
	}
	random := make([]byte, 5)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	name := fmt.Sprintf("arcvpn-state-%s-%s.json", time.Now().UTC().Format("20060102-150405.000000000"), hex.EncodeToString(random))
	target := filepath.Join(directory, name)
	f, err := os.CreateTemp(directory, ".arcvpn-state-*.tmp")
	if err != nil {
		return "", err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err != nil {
		f.Close()
		return "", err
	}
	if _, err = f.Write(blob); err != nil {
		f.Close()
		return "", err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return "", err
	}
	if err = f.Close(); err != nil {
		return "", err
	}
	if err = os.Rename(f.Name(), target); err != nil {
		return "", err
	}
	dir, err := os.Open(directory)
	if err == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
	if err = Prune(directory, keep); err != nil {
		return target, err
	}
	return target, nil
}
func Prune(directory string, keep int) error {
	if keep < 1 || keep > 30 {
		return errors.New("invalid retention count")
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}
	var filenames []string
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, "arcvpn-state-") || !strings.HasSuffix(name, ".json") || len(name) != len("arcvpn-state-20060102-150405.000000000-0123456789.json") || !entry.Type().IsRegular() {
			continue
		}
		filenames = append(filenames, name)
	}
	sort.Strings(filenames)
	for _, name := range filenames[:max(0, len(filenames)-keep)] {
		if err := os.Remove(filepath.Join(directory, name)); err != nil {
			return err
		}
	}
	return nil
}
