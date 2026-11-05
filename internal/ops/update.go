package ops

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"strings"
	"time"
)

type UpdateStatus struct {
	Trusted  bool   `json:"trusted"`
	Staged   bool   `json:"staged"`
	Version  string `json:"version,omitempty"`
	Accepted bool   `json:"accepted,omitempty"`
}

func Update(ctx context.Context, socket, action string) (UpdateStatus, error) {
	if socket == "" || (action != "status" && action != "verify" && action != "apply") {
		return UpdateStatus{}, errors.New("invalid update request")
	}
	d := net.Dialer{Timeout: 2 * time.Second}
	conn, err := d.DialContext(ctx, "unix", socket)
	if err != nil {
		return UpdateStatus{}, ErrUnavailable
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(15 * time.Second))
	if _, err = io.WriteString(conn, "update "+action+"\n"); err != nil {
		return UpdateStatus{}, ErrUnavailable
	}
	b, err := io.ReadAll(io.LimitReader(conn, 4097))
	if err != nil || len(b) > 4096 {
		return UpdateStatus{}, ErrUnavailable
	}
	var state UpdateStatus
	if strings.TrimSpace(string(b)) == "error" || json.Unmarshal(b, &state) != nil {
		return UpdateStatus{}, ErrUnavailable
	}
	return state, nil
}
