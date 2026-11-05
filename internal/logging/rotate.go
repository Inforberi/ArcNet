// Package logging contains a bounded local application log writer.
// It is not a replacement for host-wide systemd-journald retention policies.
package logging

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

const (
	DefaultBudgetMB = 500
	MinBudgetMB     = 50
	MaxBudgetMB     = 1024
	parts           = 5
)

type Rotator struct {
	mu     sync.Mutex
	path   string
	file   *os.File
	used   int64
	budget int64
}

func New(path string, budgetMB int) (*Rotator, error) {
	if budgetMB < MinBudgetMB || budgetMB > MaxBudgetMB {
		return nil, errors.New("invalid log budget")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	if err := validateExistingLog(path); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return nil, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	r := &Rotator{path: path, file: f, used: st.Size(), budget: int64(budgetMB) << 20}
	if err := r.pruneLocked(); err != nil {
		f.Close()
		return nil, err
	}
	return r, nil
}
func (r *Rotator) SetBudgetMB(value int) error {
	if value < MinBudgetMB || value > MaxBudgetMB {
		return errors.New("log budget outside allowed range")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.budget = int64(value) << 20
	return r.pruneLocked()
}
func (r *Rotator) BudgetMB() int { r.mu.Lock(); defer r.mu.Unlock(); return int(r.budget >> 20) }
func (r *Rotator) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.file == nil {
		return 0, os.ErrClosed
	}
	size := int64(len(p))
	limit := r.budget / parts
	if size > limit {
		return 0, errors.New("log entry exceeds maximum segment size")
	}
	if r.used+size > limit {
		if err := r.rotate(); err != nil {
			return 0, err
		}
	}
	n, err := r.file.Write(p)
	r.used += int64(n)
	return n, err
}
func (r *Rotator) rotate() error {
	if err := r.file.Close(); err != nil {
		return err
	}
	r.file = nil
	_ = os.Remove(fmt.Sprintf("%s.%d", r.path, parts-1))
	for i := parts - 2; i >= 1; i-- {
		from := fmt.Sprintf("%s.%d", r.path, i)
		to := fmt.Sprintf("%s.%d", r.path, i+1)
		if err := os.Rename(from, to); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if err := os.Rename(r.path, r.path+".1"); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	f, err := os.OpenFile(r.path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	r.file = f
	r.used = 0
	return nil
}

// pruneLocked removes only archived application segments first. Existing legacy
// oversized active files are cleared as a last resort so the configured cap is
// enforced immediately; it never touches journald or unrelated files.
func (r *Rotator) pruneLocked() error {
	for i := 1; i < parts; i++ {
		path := fmt.Sprintf("%s.%d", r.path, i)
		if err := validateExistingLog(path); err != nil {
			return err
		}
	}
	for {
		total := r.used
		for i := 1; i < parts; i++ {
			if st, err := os.Stat(fmt.Sprintf("%s.%d", r.path, i)); err == nil {
				total += st.Size()
			}
		}
		if total <= r.budget {
			return nil
		}
		removed := false
		for i := parts - 1; i >= 1; i-- {
			path := fmt.Sprintf("%s.%d", r.path, i)
			if _, err := os.Stat(path); err == nil {
				if err = os.Remove(path); err != nil {
					return err
				}
				removed = true
				break
			}
		}
		if !removed {
			if r.file == nil {
				return os.ErrClosed
			}
			if err := r.file.Truncate(0); err != nil {
				return err
			}
			r.used = 0
			return nil
		}
	}
}
func validateExistingLog(path string) error {
	st, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !st.Mode().IsRegular() {
		return fmt.Errorf("log path must be regular file: %s", path)
	}
	return nil
}
func (r *Rotator) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.file == nil {
		return nil
	}
	err := r.file.Close()
	r.file = nil
	return err
}

var _ io.Writer = (*Rotator)(nil)

// Tail returns at most n bytes from the active application log, never from
// arbitrary paths supplied by the HTTP client.
func (r *Rotator) Tail(n int64) (string, error) {
	if n <= 0 || n > 64<<10 {
		return "", errors.New("invalid log tail length")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	f, err := os.Open(r.path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return "", err
	}
	start := st.Size() - n
	if start < 0 {
		start = 0
	}
	buf := make([]byte, st.Size()-start)
	_, err = f.ReadAt(buf, start)
	if err != nil && err != io.EOF {
		return "", err
	}
	return string(buf), nil
}
func (r *Rotator) UsedBytes() int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	var total int64
	for i := 0; i < parts; i++ {
		p := r.path
		if i > 0 {
			p = fmt.Sprintf("%s.%d", r.path, i)
		}
		if st, err := os.Stat(p); err == nil {
			total += st.Size()
		}
	}
	return total
}
func (r *Rotator) Clear() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.file == nil {
		return os.ErrClosed
	}
	if err := r.file.Truncate(0); err != nil {
		return err
	}
	if _, err := r.file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	r.used = 0
	for i := 1; i < parts; i++ {
		if err := os.Remove(fmt.Sprintf("%s.%d", r.path, i)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}
