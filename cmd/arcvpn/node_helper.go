package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/user"
	"strconv"
	"syscall"
	"time"

	"github.com/arcvpn/arcvpn/internal/noderuntime"
)

// The node helper is socket-activated by systemd and must never bind its own
// world-accessible socket. On every request, verify the kernel's peer UID.
func runNodeHelper() error {
	if os.Geteuid() != 0 {
		return errors.New("node-helper must run as root")
	}
	account, err := user.Lookup("arcvpn-node")
	if err != nil {
		return err
	}
	expected, err := strconv.Atoi(account.Uid)
	if err != nil {
		return err
	}
	f := os.NewFile(3, "systemd-node-socket")
	if f == nil {
		return errors.New("missing systemd node socket")
	}
	defer f.Close()
	listener, err := net.FileListener(f)
	if err != nil {
		return err
	}
	defer listener.Close()
	unix, ok := listener.(*net.UnixListener)
	if !ok {
		return errors.New("expected Unix socket")
	}
	for {
		_ = unix.SetDeadline(time.Now().Add(45 * time.Second))
		connection, acceptErr := unix.AcceptUnix()
		if acceptErr != nil {
			if n, ok := acceptErr.(net.Error); ok && n.Timeout() {
				return nil
			}
			return acceptErr
		}
		func() {
			defer connection.Close()
			raw, err := connection.SyscallConn()
			if err != nil {
				return
			}
			var peerUID uint32
			var peerErr error
			if err = raw.Control(func(fd uintptr) {
				var cred *syscall.Ucred
				cred, peerErr = syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
				if peerErr == nil {
					peerUID = cred.Uid
				}
			}); err != nil || peerErr != nil || int(peerUID) != expected {
				return
			}
			ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
			defer cancel()
			noderuntime.ServeConn(ctx, connection, noderuntime.Installer{})
		}()
	}
}

func nodeHelperPreflight() error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("node-helper requires root")
	}
	return nil
}
