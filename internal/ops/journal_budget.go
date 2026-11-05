package ops

import (
	"context"
	"errors"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

// JournalBudget reads/updates only the ArcVPN journal namespace. The privileged
// helper enforces its own allowlist and filesystem boundaries independently.
func JournalBudget(ctx context.Context, socket string, newLimit int) (int, error) {
	if socket == "" || (newLimit != 0 && (newLimit < 50 || newLimit > 1024)) {
		return 0, ErrUnavailable
	}
	command := "journal-budget status\n"
	if newLimit != 0 {
		command = "journal-budget " + strconv.Itoa(newLimit) + "\n"
	}
	d := net.Dialer{Timeout: 2 * time.Second}
	conn, err := d.DialContext(ctx, "unix", socket)
	if err != nil {
		return 0, ErrUnavailable
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(12 * time.Second))
	if _, err = io.WriteString(conn, command); err != nil {
		return 0, ErrUnavailable
	}
	out, err := io.ReadAll(io.LimitReader(conn, 64))
	if err != nil || len(out) > 16 {
		return 0, ErrUnavailable
	}
	text := strings.TrimSpace(string(out))
	number, err := strconv.Atoi(text)
	if err != nil || number < 50 || number > 1024 {
		return 0, errors.New("invalid journal budget response")
	}
	return number, nil
}
