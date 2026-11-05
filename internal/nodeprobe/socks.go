package nodeprobe

import (
	"context"
	"errors"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

type socksDialer struct{ address, username, password string }

// DialContext implements the strict minimum of RFC 1928 and RFC 1929.
// The requested destination is forwarded by the proxy; it is never dialed
// from the machine running the probe itself.
func (d *socksDialer) DialContext(ctx context.Context, network, destination string) (net.Conn, error) {
	if network != "tcp" {
		return nil, errors.New("SOCKS probe supports TCP only")
	}
	hostname, p, err := net.SplitHostPort(destination)
	if err != nil {
		return nil, err
	}
	port, err := strconv.Atoi(p)
	if err != nil || port <= 0 || port > 65535 || len(hostname) == 0 || len(hostname) > 253 || strings.ContainsAny(hostname, "\x00\r\n") {
		return nil, errors.New("invalid SOCKS destination")
	}
	if len(d.username) > 255 || len(d.password) > 255 || len(d.username) == 0 || len(d.password) == 0 {
		return nil, errors.New("invalid SOCKS authentication")
	}
	c, err := (&net.Dialer{}).DialContext(ctx, "tcp", d.address)
	if err != nil {
		return nil, err
	}
	ok := false
	stop := context.AfterFunc(ctx, func() { _ = c.Close() })
	defer stop()
	defer func() {
		if !ok {
			_ = c.Close()
		}
	}()
	if deadline, hasDeadline := ctx.Deadline(); hasDeadline {
		_ = c.SetDeadline(deadline)
	}
	if _, err = c.Write([]byte{5, 1, 2}); err != nil {
		return nil, err
	}
	response := make([]byte, 2)
	if _, err = io.ReadFull(c, response); err != nil {
		return nil, err
	}
	if response[0] != 5 || response[1] != 2 {
		return nil, errors.New("SOCKS authentication method refused")
	}
	auth := []byte{1, byte(len(d.username))}
	auth = append(auth, []byte(d.username)...)
	auth = append(auth, byte(len(d.password)))
	auth = append(auth, []byte(d.password)...)
	if _, err = c.Write(auth); err != nil {
		return nil, err
	}
	if _, err = io.ReadFull(c, response); err != nil {
		return nil, err
	}
	if response[0] != 1 || response[1] != 0 {
		return nil, errors.New("SOCKS credentials rejected")
	}
	request := []byte{5, 1, 0, 3, byte(len(hostname))}
	request = append(request, []byte(hostname)...)
	request = append(request, byte(port>>8), byte(port))
	if _, err = c.Write(request); err != nil {
		return nil, err
	}
	reply := make([]byte, 4)
	if _, err = io.ReadFull(c, reply); err != nil {
		return nil, err
	}
	if reply[0] != 5 || reply[1] != 0 || reply[2] != 0 {
		return nil, errors.New("SOCKS CONNECT refused")
	}
	var n int
	switch reply[3] {
	case 1:
		n = 4
	case 4:
		n = 16
	case 3:
		one := make([]byte, 1)
		if _, err = io.ReadFull(c, one); err != nil {
			return nil, err
		}
		n = int(one[0])
	default:
		return nil, errors.New("invalid SOCKS response address type")
	}
	tail := make([]byte, n+2)
	if _, err = io.ReadFull(c, tail); err != nil {
		return nil, err
	}
	_ = c.SetDeadline(time.Time{})
	ok = true
	return c, nil
}
