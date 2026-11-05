// Package ops contains bounded calls to the restricted local control socket.
package ops

import (
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"time"
)

var ErrUnavailable = errors.New("local privileged helper unavailable")
var services = map[string]bool{"panel": true, "xray": true, "hysteria": true, "wireguard": true, "amneziawg": true}

func JournalTail(ctx context.Context, socket, service string) (string, error) {
	if !services[service] || socket == "" {
		return "", ErrUnavailable
	}
	dialer := net.Dialer{Timeout: 2 * time.Second}
	conn, err := dialer.DialContext(ctx, "unix", socket)
	if err != nil {
		return "", ErrUnavailable
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err = io.WriteString(conn, "journal "+service+"\n"); err != nil {
		return "", ErrUnavailable
	}
	out, err := io.ReadAll(io.LimitReader(conn, 32<<10))
	if err != nil || len(out) == 0 || len(out) >= 32<<10 || string(out) == "error\n" {
		return "", ErrUnavailable
	}
	return strings.ToValidUTF8(string(out), "�"), nil
}
