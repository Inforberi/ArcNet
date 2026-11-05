package nodeprobe

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const probeUUID = "11111111-2222-4333-8444-555555555555"

func exampleLink() string {
	key := base64.RawURLEncoding.EncodeToString(bytes32())
	return fmt.Sprintf("vless://%s@9.9.9.9:443?encryption=none&security=reality&sni=www.example.com&fp=chrome&pbk=%s&sid=abcdef1234567890&type=tcp&flow=xtls-rprx-vision#probe", probeUUID, key)
}
func bytes32() []byte { return []byte("01234567890123456789012345678901") }

func TestStrictLinkValidation(t *testing.T) {
	valid := exampleLink()
	parsed, err := ParseLink(valid)
	if err != nil || parsed.Address != "9.9.9.9" || parsed.Port != 443 || parsed.UUID != probeUUID {
		t.Fatalf("unexpected link: %+v %v", parsed, err)
	}
	for label, change := range map[string]string{
		"private IP":  "127.0.0.1:443",
		"metadata IP": "169.254.169.254:443",
		"reserved IP": "203.0.113.8:443",
		"wrong port":  "9.9.9.9:0",
	} {
		altered := strings.Replace(valid, "9.9.9.9:443", change, 1)
		if _, err = ParseLink(altered); err == nil {
			t.Errorf("accepted %s", label)
		}
	}
	for label, change := range map[string]string{
		"plaintext": "security=none", "wrong flow": "flow=", "wrong transport": "type=ws", "wrong fingerprint": "fp=firefox", "bad shortid": "sid=xyz", "bad key": "pbk=not-a-key", "bad UUID": "vless://invalid@",
	} {
		original := ""
		switch label {
		case "plaintext":
			original = "security=reality"
		case "wrong flow":
			original = "flow=xtls-rprx-vision"
		case "wrong transport":
			original = "type=tcp"
		case "wrong fingerprint":
			original = "fp=chrome"
		case "bad shortid":
			original = "sid=abcdef1234567890"
		case "bad key":
			original = "pbk=" + parsed.PublicKey
		case "bad UUID":
			original = "vless://" + probeUUID + "@"
		}
		if _, err = ParseLink(strings.Replace(valid, original, change, 1)); err == nil {
			t.Errorf("accepted %s", label)
		}
	}
	if _, err = ParseLink(strings.Replace(valid, "#probe", "&security=reality#probe", 1)); err == nil {
		t.Fatal("duplicate parameter accepted")
	}
	if _, err = ParseLink(strings.Replace(valid, "#probe", "&arbitrary=1#probe", 1)); err == nil {
		t.Fatal("unknown parameter accepted")
	}
}

func TestProbeConfigHasNoDirectBypassAndNoExposedListener(t *testing.T) {
	link, err := ParseLink(exampleLink())
	if err != nil {
		t.Fatal(err)
	}
	raw, err := Config(link, 17890, "private-user", "private-password")
	if err != nil {
		t.Fatal(err)
	}
	var c struct {
		Inbounds []struct {
			Listen   string `json:"listen"`
			Port     int    `json:"port"`
			Settings struct {
				Auth  string `json:"auth"`
				Users []struct {
					User string `json:"user"`
					Pass string `json:"pass"`
				} `json:"users"`
			} `json:"settings"`
		} `json:"inbounds"`
		Outbounds []struct {
			Protocol       string `json:"protocol"`
			StreamSettings struct {
				Security string `json:"security"`
				Reality  struct {
					Password string `json:"password"`
				} `json:"realitySettings"`
			} `json:"streamSettings"`
		} `json:"outbounds"`
	}
	if err = json.Unmarshal(raw, &c); err != nil {
		t.Fatal(err)
	}
	if len(c.Inbounds) != 1 || c.Inbounds[0].Listen != "127.0.0.1" || c.Inbounds[0].Port != 17890 || c.Inbounds[0].Settings.Auth != "password" || len(c.Inbounds[0].Settings.Users) != 1 || c.Inbounds[0].Settings.Users[0].Pass != "private-password" {
		t.Fatalf("unsafe listener: %s", raw)
	}
	if len(c.Outbounds) != 1 || c.Outbounds[0].Protocol != "vless" || c.Outbounds[0].StreamSettings.Security != "reality" || c.Outbounds[0].StreamSettings.Reality.Password != link.PublicKey {
		t.Fatalf("missing authenticated VLESS-only outbound: %s", raw)
	}
	if strings.Contains(string(raw), `"protocol":"freedom"`) {
		t.Fatal("direct egress fallback")
	}
}

func TestXrayExitedOrRejectedConfigCannotPass(t *testing.T) {
	for _, scenario := range []struct{ name, script string }{{"reject", "#!/bin/sh\nexit 1\n"}, {"immediate exit", "#!/bin/sh\nexit 0\n"}} {
		t.Run(scenario.name, func(t *testing.T) {
			binary := filepath.Join(t.TempDir(), "fake-xray")
			if err := os.WriteFile(binary, []byte(scenario.script), 0700); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			result, err := Run(ctx, binary, exampleLink())
			if err == nil || result.Result == "pass" {
				t.Fatalf("fake client passed: %+v %v", result, err)
			}
		})
	}
}

func TestAuthenticatedSOCKSConnectUsesProxyInsteadOfDirectNetwork(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	done := make(chan error, 1)
	go func() {
		conn, e := listener.Accept()
		if e != nil {
			done <- e
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
		greeting := make([]byte, 3)
		if _, e = io.ReadFull(conn, greeting); e != nil {
			done <- e
			return
		}
		if string(greeting) != string([]byte{5, 1, 2}) {
			done <- errors.New("unexpected SOCKS methods")
			return
		}
		_, _ = conn.Write([]byte{5, 2})
		prefix := make([]byte, 2)
		if _, e = io.ReadFull(conn, prefix); e != nil {
			done <- e
			return
		}
		username := make([]byte, int(prefix[1]))
		if _, e = io.ReadFull(conn, username); e != nil {
			done <- e
			return
		}
		passLength := make([]byte, 1)
		if _, e = io.ReadFull(conn, passLength); e != nil {
			done <- e
			return
		}
		password := make([]byte, int(passLength[0]))
		if _, e = io.ReadFull(conn, password); e != nil {
			done <- e
			return
		}
		if string(username) != "probe-user" || string(password) != "probe-pass" {
			done <- errors.New("authentication failed")
			return
		}
		_, _ = conn.Write([]byte{1, 0})
		request := make([]byte, 5)
		if _, e = io.ReadFull(conn, request); e != nil {
			done <- e
			return
		}
		hostname := make([]byte, int(request[4]))
		if _, e = io.ReadFull(conn, hostname); e != nil {
			done <- e
			return
		}
		port := make([]byte, 2)
		if _, e = io.ReadFull(conn, port); e != nil {
			done <- e
			return
		}
		if string(hostname) != "example.org" || port[0] != 1 || port[1] != 187 {
			done <- errors.New("wrong tunnel destination")
			return
		}
		_, _ = conn.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, 0, 1})
		done <- nil
	}()
	dialer := &socksDialer{address: listener.Addr().String(), username: "probe-user", password: "probe-pass"}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, err := dialer.DialContext(ctx, "tcp", "example.org:443")
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.Close()
	if err = <-done; err != nil {
		t.Fatal(err)
	}
}
