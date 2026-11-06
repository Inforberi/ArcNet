// Package lifecycle coordinates explicit local service operations. It does not
// accept unit names, command lines or paths from an HTTP caller.
package lifecycle

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

const Directory = "/run/arcvpn-lifecycle"

var ErrPaused = errors.New("local runtime is stopped or undergoing maintenance; use sudo arcvpn start")
var ErrBusy = errors.New("another local lifecycle or update operation is running")

type Guard struct{ Dir string }

func (g Guard) directory() string {
	if g.Dir == "" {
		return Directory
	}
	return g.Dir
}

func rootFile(info os.FileInfo) bool {
	st, ok := info.Sys().(*syscall.Stat_t)
	return ok && st.Uid == 0 && info.Mode().Perm()&0022 == 0
}

func (g Guard) Check() error {
	info, err := os.Lstat(g.directory())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || !info.IsDir() || !rootFile(info) {
		return errors.New("unsafe lifecycle directory")
	}
	info, err = os.Lstat(filepath.Join(g.directory(), "paused"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || !info.Mode().IsRegular() || !rootFile(info) {
		return errors.New("unsafe lifecycle marker")
	}
	return ErrPaused
}

// Lock is root-only and nonblocking. Its directory must outlive any unit;
// unlinking a locked inode would break mutual exclusion with the updater.
func (g Guard) Lock() (func(), error) {
	if os.Geteuid() != 0 {
		return nil, errors.New("local lifecycle requires root")
	}
	dir := g.directory()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || !rootFile(info) {
		return nil, errors.New("unsafe lifecycle directory")
	}
	// The unprivileged controller must be able to inspect the marker even
	// when the root shell has umask 077. Lock contents remain root-only.
	if info.Mode().Perm() != 0755 {
		if err := os.Chmod(dir, 0755); err != nil {
			return nil, err
		}
	}
	fd, err := syscall.Open(filepath.Join(dir, "lock"), syscall.O_CREAT|syscall.O_RDWR|syscall.O_CLOEXEC|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0600)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), "lifecycle-lock")
	info, err = f.Stat()
	if err != nil || !info.Mode().IsRegular() || !rootFile(info) {
		f.Close()
		return nil, errors.New("unsafe lifecycle lock")
	}
	if err := syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, ErrBusy
	}
	return func() { _ = syscall.Flock(fd, syscall.LOCK_UN); _ = f.Close() }, nil
}

func (g Guard) Pause() error {
	// Called only while holding Lock. Atomic replacement never follows a link.
	f, err := os.CreateTemp(g.directory(), ".paused-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err := f.Chmod(0644); err != nil {
		f.Close()
		return err
	}
	if _, err := f.WriteString("stopped\n"); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), filepath.Join(g.directory(), "paused"))
}

func (g Guard) Resume() error {
	if err := g.Check(); err != nil && !errors.Is(err, ErrPaused) {
		return err
	}
	err := os.Remove(filepath.Join(g.directory(), "paused"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
