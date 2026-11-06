package updater

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"syscall"
	"time"

	"github.com/arcvpn/arcvpn/internal/lifecycle"
)

type Exec interface {
	Run(context.Context, string, ...string) error
}
type commandRunner struct{}

func (commandRunner) Run(ctx context.Context, name string, args ...string) error {
	c := exec.CommandContext(ctx, name, args...)
	c.Stdout = os.Stderr
	c.Stderr = os.Stderr
	return c.Run()
}

type Installer struct {
	Binary       string
	Staged       string
	PublicKey    string
	LockFile     string
	Runner       Exec
	Coordination *lifecycle.Guard
}

func (i Installer) defaults() Installer {
	if i.Binary == "" {
		i.Binary = CoreBinary
	}
	if i.Staged == "" {
		i.Staged = StagedArchive
	}
	if i.PublicKey == "" {
		i.PublicKey = PublicKeyFile
	}
	if i.LockFile == "" {
		i.LockFile = "/run/arcvpn-update/lock"
	}
	if i.Runner == nil {
		i.Runner = commandRunner{}
	}
	return i
}

// Read through one descriptor and refuse symlinks, special files and oversized
// archives before allocating memory. The staged path is writable by the
// unprivileged panel, while this verifier executes as root.
func readStagedArchive(path string) ([]byte, error) {
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, ErrInvalid
	}
	f := os.NewFile(uintptr(fd), path)
	defer f.Close()
	stat, err := f.Stat()
	if err != nil || !stat.Mode().IsRegular() || stat.Size() == 0 || stat.Size() > MaxArchive {
		return nil, ErrInvalid
	}
	data, err := io.ReadAll(io.LimitReader(f, MaxArchive+1))
	if err != nil || len(data) == 0 || len(data) > MaxArchive {
		return nil, ErrInvalid
	}
	return data, nil
}

func (i Installer) Verify() (Manifest, error) {
	i = i.defaults()
	pub, err := ReadPublicKey(i.PublicKey)
	if err != nil {
		return Manifest{}, err
	}
	b, err := readStagedArchive(i.Staged)
	if err != nil {
		return Manifest{}, err
	}
	payload, err := Verify(b, pub)
	if err != nil {
		return Manifest{}, err
	}
	return payload.Manifest, nil
}
func (i Installer) Apply(ctx context.Context) error {
	i = i.defaults()
	pub, err := ReadPublicKey(i.PublicKey)
	if err != nil {
		return err
	}
	guard := lifecycle.Guard{}
	if i.Coordination != nil {
		guard = *i.Coordination
	}
	unlock, err := guard.Lock()
	if err != nil {
		return err
	}
	defer unlock()
	if err := guard.Check(); err != nil {
		return err
	}
	lock, err := os.OpenFile(i.LockFile, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return errors.New("another update is already running")
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	blob, err := readStagedArchive(i.Staged)
	if err != nil {
		return err
	}
	payload, err := Verify(blob, pub)
	if err != nil {
		return err
	}
	binaryDir := filepath.Dir(i.Binary)
	data := map[string][]byte{"arcvpn-core": payload.Binary}
	for name, b := range payload.Components {
		data[name] = b
	}
	type change struct {
		target, backup, candidate string
		existed                   bool
		installed                 bool
	}
	changes := make([]*change, 0, len(data))
	defer func() {
		for _, c := range changes {
			if c.candidate != "" {
				_ = os.Remove(c.candidate)
			}
		}
	}()
	keys := make([]string, 0, len(data))
	for name := range data {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	for _, name := range keys {
		target := filepath.Join(binaryDir, name)
		if name == "arcvpn-core" {
			target = i.Binary
		}
		st, statErr := os.Lstat(target)
		if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
			return statErr
		}
		if statErr == nil && !st.Mode().IsRegular() {
			return errors.New("installed component is not a regular file")
		}
		candidate, err := os.CreateTemp(binaryDir, ".arcvpn-candidate-*")
		if err != nil {
			return err
		}
		c := &change{target: target, backup: target + ".previous", candidate: candidate.Name(), existed: statErr == nil}
		changes = append(changes, c)
		if _, err = candidate.Write(data[name]); err != nil {
			candidate.Close()
			return err
		}
		if err = candidate.Chmod(0755); err != nil {
			candidate.Close()
			return err
		}
		if err = candidate.Sync(); err != nil {
			candidate.Close()
			return err
		}
		if err = candidate.Close(); err != nil {
			return err
		}
		if name == "arcvpn-core" {
			checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			err = i.Runner.Run(checkCtx, c.candidate, "frontend-check")
			cancel()
			if err != nil {
				return fmt.Errorf("update candidate missing React: %w", err)
			}
		}
	}
	rollback := func() error {
		var errs []error
		for j := len(changes) - 1; j >= 0; j-- {
			c := changes[j]
			if !c.installed {
				continue
			}
			_ = os.Remove(c.target)
			if c.existed {
				if err := os.Rename(c.backup, c.target); err != nil {
					errs = append(errs, err)
				}
			}
		}
		return errors.Join(errs...)
	}
	for _, c := range changes {
		if c.existed {
			if err = os.Remove(c.backup); err != nil && !errors.Is(err, os.ErrNotExist) {
				_ = rollback()
				return err
			}
			if err = os.Rename(c.target, c.backup); err != nil {
				_ = rollback()
				return err
			}
		}
		if err = os.Rename(c.candidate, c.target); err != nil {
			if c.existed {
				_ = os.Rename(c.backup, c.target)
			}
			if undoErr := rollback(); undoErr != nil {
				return fmt.Errorf("update install failed (%v); rollback failed: %w", err, undoErr)
			}
			return err
		}
		c.candidate = ""
		c.installed = true
	}
	restartErr := i.restartChanged(ctx, payload.Components)
	if restartErr == nil {
		_ = os.Remove(i.Staged)
		return nil
	}
	recoveryCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	rollbackErr := rollback()
	if rollbackErr != nil {
		return fmt.Errorf("update failed (%v) and file rollback failed: %w", restartErr, rollbackErr)
	}
	if serviceErr := i.restartChanged(recoveryCtx, payload.Components); serviceErr != nil {
		return fmt.Errorf("new release failed (%v), old binaries restored but service recovery failed: %w", restartErr, serviceErr)
	}
	return fmt.Errorf("new release failed; previous version restored: %w", restartErr)
}

func (i Installer) restartChanged(ctx context.Context, components map[string][]byte) error {
	units := map[string]bool{}
	for name := range components {
		switch name {
		case "xray":
			units["arcvpn-xray.service"] = true
		case "hysteria":
			units["arcvpn-hysteria.service"] = true
		case "awg", "awg-quick", "amneziawg-go":
			units["arcvpn-amneziawg.service"] = true
		}
	}
	sorted := make([]string, 0, len(units))
	for unit := range units {
		sorted = append(sorted, unit)
	}
	sort.Strings(sorted)
	for _, unit := range sorted {
		// Do not start a VPN engine which has no configured listener. Only restart
		// those already active when an engine binary changes.
		if err := i.Runner.Run(ctx, "/usr/bin/systemctl", "is-active", "--quiet", unit); err != nil {
			continue
		}
		if err := i.Runner.Run(ctx, "/usr/bin/systemctl", "restart", unit); err != nil {
			return err
		}
		if err := i.Runner.Run(ctx, "/usr/bin/systemctl", "is-active", "--quiet", unit); err != nil {
			return err
		}
	}
	return i.restartAndCheck(ctx)
}
func (i Installer) restartAndCheck(ctx context.Context) error {
	if err := i.Runner.Run(ctx, "/usr/bin/systemctl", "restart", "arcvpn-panel.service"); err != nil {
		return err
	}
	for attempt := 0; attempt < 3; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(700 * time.Millisecond):
		}
		if err := i.Runner.Run(ctx, "/usr/bin/systemctl", "is-active", "--quiet", "arcvpn-panel.service"); err != nil {
			return err
		}
	}
	return nil
}
