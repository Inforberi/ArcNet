// Package testfixture contains isolated real-process helpers for VPN acceptance
// tests only. It is never imported by the server or privileged helpers.
package testfixture

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

type Process struct {
	Cmd  *exec.Cmd
	done chan struct{}
	once sync.Once
}

func (p *Process) Stop() {
	p.once.Do(func() {
		_ = p.Cmd.Process.Signal(syscall.SIGTERM)
		select {
		case <-p.done:
		case <-time.After(3 * time.Second):
			_ = p.Cmd.Process.Kill()
			<-p.done
		}
	})
}
func Start(t *testing.T, binary string, args ...string) *Process {
	t.Helper()
	cmd := exec.Command("sh", append([]string{"-c", `umask 077; exec "$@"`, "vpn-fixture", binary}, args...)...)
	var diagnostics bytes.Buffer
	cmd.Stderr = &diagnostics
	if err := cmd.Start(); err != nil {
		t.Fatal("start isolated runtime", err)
	}
	p := &Process{Cmd: cmd, done: make(chan struct{})}
	go func() { _ = cmd.Wait(); close(p.done) }()
	t.Cleanup(p.Stop)
	t.Cleanup(func() {
		p.Stop()
		if t.Failed() {
			// Only JSON error fields from test-only runtimes; never dump configs.
			for _, line := range strings.Split(diagnostics.String(), "\n") {
				var entry struct {
					Message string `json:"msg"`
					Error   string `json:"error"`
				}
				if json.Unmarshal([]byte(line), &entry) == nil && entry.Error != "" {
					t.Logf("isolated runtime: %s (%s)", entry.Message, entry.Error)
				}
			}
		}
	})
	return p
}
func WriteJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}
func Port(t *testing.T, address string) int {
	t.Helper()
	l, err := net.Listen("tcp", net.JoinHostPort(address, "0"))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}
func Wait(t *testing.T, description string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(40 * time.Millisecond)
	}
	t.Fatal("timed out:", description)
}
func Echo(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	connections := map[net.Conn]bool{}
	t.Cleanup(func() {
		_ = l.Close()
		mu.Lock()
		defer mu.Unlock()
		for c := range connections {
			_ = c.Close()
		}
	})
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			connections[c] = true
			mu.Unlock()
			go func() { defer c.Close(); _, _ = io.Copy(c, c); mu.Lock(); delete(connections, c); mu.Unlock() }()
		}
	}()
	return l.Addr().String()
}
func SOCKS(t *testing.T, proxyPort int, target string) net.Conn {
	t.Helper()
	c, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(proxyPort)), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err = c.Write([]byte{5, 1, 0}); err != nil {
		t.Fatal(err)
	}
	reply := make([]byte, 2)
	if _, err = io.ReadFull(c, reply); err != nil || reply[1] != 0 {
		t.Fatal("SOCKS authentication", err)
	}
	host, portText, err := net.SplitHostPort(target)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	request := append([]byte{5, 1, 0, 1}, net.ParseIP(host).To4()...)
	request = binary.BigEndian.AppendUint16(request, uint16(port))
	if _, err = c.Write(request); err != nil {
		t.Fatal(err)
	}
	header := make([]byte, 4)
	if _, err = io.ReadFull(c, header); err != nil || header[1] != 0 {
		t.Fatal("SOCKS connection", err)
	}
	size := 4
	switch header[3] {
	case 1:
		size = 4
	case 4:
		size = 16
	case 3:
		b := make([]byte, 1)
		if _, err = io.ReadFull(c, b); err != nil {
			t.Fatal(err)
		}
		size = int(b[0])
	default:
		t.Fatal("SOCKS address format")
	}
	if _, err = io.ReadFull(c, make([]byte, size+2)); err != nil {
		t.Fatal(err)
	}
	_ = c.SetDeadline(time.Time{})
	return c
}
func Transfer(t *testing.T, c net.Conn) {
	t.Helper()
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := c.Write([]byte("presence-proof")); err != nil {
		t.Fatal(err)
	}
	data := make([]byte, len("presence-proof"))
	if _, err := io.ReadFull(c, data); err != nil || string(data) != "presence-proof" {
		t.Fatal("real proxy transfer failed", err)
	}
	_ = c.SetDeadline(time.Time{})
}
