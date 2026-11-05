package ops

import (
	"bufio"
	"context"
	"errors"
	"net"
	"path/filepath"
	"testing"
)

func TestJournalTailAllowlist(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "helper.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	done := make(chan string, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			done <- "accept error"
			return
		}
		defer conn.Close()
		command, err := bufio.NewReader(conn).ReadString('\n')
		if err != nil {
			done <- "read error"
			return
		}
		done <- command
		_, _ = conn.Write([]byte("arcvpn-xray.service started\n"))
	}()
	content, err := JournalTail(context.Background(), socket, "xray")
	if err != nil || content != "arcvpn-xray.service started\n" {
		t.Fatal(content, err)
	}
	if got := <-done; got != "journal xray\n" {
		t.Fatal(got)
	}
	if _, err := JournalTail(context.Background(), socket, "../../etc/passwd"); !errors.Is(err, ErrUnavailable) {
		t.Fatal("accepted arbitrary service", err)
	}
}
