package main

import (
	"errors"
	"testing"
)

func TestParseSystemdStateFailsClosed(t *testing.T) {
	cases := []struct {
		name    string
		value   string
		cmdErr  error
		want    string
		wantErr bool
	}{
		{name: "running", value: "active\n", want: "active"},
		{name: "stopped", value: "inactive\n", cmdErr: errors.New("exit 3"), want: "inactive"},
		{name: "crashed", value: "failed\n", cmdErr: errors.New("exit 3"), want: "inactive"},
		{name: "unit missing", value: "unknown\n", cmdErr: errors.New("exit 4"), wantErr: true},
		{name: "systemd unavailable", value: "Failed to connect to bus\n", cmdErr: errors.New("exit 1"), wantErr: true},
		{name: "inconsistent status", value: "active\n", cmdErr: errors.New("exit 1"), wantErr: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := parseSystemdState([]byte(c.value), c.cmdErr)
			if got != c.want || (err != nil) != c.wantErr {
				t.Fatalf("state=%q err=%v, expected %q err=%v", got, err, c.want, c.wantErr)
			}
		})
	}
}
