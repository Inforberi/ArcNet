package engines

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const (
	awgBinary      = "/opt/arcvpn/bin/awg"
	awgQuickBinary = "/opt/arcvpn/bin/awg-quick"
	awgGoBinary    = "/opt/arcvpn/bin/amneziawg-go"
	ipBinary       = "/usr/sbin/ip"
	awgInterface   = "arcawg"
	awgAddress     = "10.77.0.1/16"
)

// runAmneziaUserspace never asks the kernel to create an AmneziaWG interface.
// Running the pinned 3.1 userspace daemon directly prevents an older loaded
// AWG kernel module from silently taking over the interface.
// The source has already passed ValidateQuickServer, is root-owned, and is
// stored under /run/arcvpn with mode 0600.
func runAmneziaUserspace(ctx context.Context, action, source string) error {
	if action == "down" {
		err := commandOutput(ctx, ipBinary, "link", "del", "dev", awgInterface)
		if err != nil {
			if _, statErr := os.Stat(filepath.Join("/sys/class/net", awgInterface)); !os.IsNotExist(statErr) {
				return err
			}
		}
		return nil
	}
	if action != "up" {
		return errors.New("invalid AmneziaWG operation")
	}
	// awg-quick 'strip' is a non-executing parser. The result contains only
	// native awg interface/peer parameters, not Address or shell hooks.
	stripCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	clean, err := awgCommand(stripCtx, awgQuickBinary, "strip", source).Output()
	cancel()
	if err != nil || len(clean) == 0 || len(clean) > 1<<20 {
		return fmt.Errorf("AmneziaWG strip failed or returned invalid data: %w", err)
	}
	// The caller creates /run/arcvpn as root-only; never expose a secret file.
	f, err := os.CreateTemp("/run/arcvpn", ".arcawg-native-*.conf")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err := f.Chmod(0600); err != nil {
		_ = f.Close()
		return err
	}
	_, writeErr := f.Write(clean)
	closeErr := f.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		return err
	}
	if err := commandOutput(ctx, awgGoBinary, awgInterface); err != nil {
		return err
	}
	ready := false
	defer func() {
		if !ready {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = commandOutput(cleanupCtx, ipBinary, "link", "del", "dev", awgInterface)
		}
	}()
	// The userspace daemon can fork before its UAPI socket becomes ready.
	for attempt := 0; attempt < 10; attempt++ {
		if commandOutput(ctx, awgBinary, "show", awgInterface) == nil {
			ready = true
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	if !ready {
		return errors.New("AmneziaWG 3.1 userspace daemon did not become ready")
	}
	// No arbitrary wg-quick shell hooks, routes or DNS scripts are executed.
	if err := commandOutput(ctx, awgBinary, "setconf", awgInterface, name); err != nil {
		ready = false
		return err
	}
	for _, args := range [][]string{
		{"address", "add", awgAddress, "dev", awgInterface},
		{"link", "set", "dev", awgInterface, "mtu", "1280"},
		{"link", "set", "dev", awgInterface, "up"},
	} {
		if err := commandOutput(ctx, ipBinary, args...); err != nil {
			ready = false
			return err
		}
	}
	return nil
}

func awgCommand(ctx context.Context, program string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, program, args...)
	cmd.Env = []string{
		"PATH=/opt/arcvpn/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"HOME=/root", "LANG=C",
	}
	return cmd
}

func commandOutput(ctx context.Context, program string, args ...string) error {
	out, err := awgCommand(ctx, program, args...).CombinedOutput()
	if err != nil {
		// Some upstream errors may echo the sensitive configuration. Do not
		// include command output in errors returned to a browser or journal.
		_ = out
		return fmt.Errorf("AmneziaWG command %s failed: %w", filepath.Base(program), err)
	}
	return nil
}
