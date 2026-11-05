package engines

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ValidateQuickServer is a strict, command-free wg-quick/awg-quick grammar.
// The resulting root-owned staging file is safe to pass to wg-quick, whose
// arbitrary PreUp/PostUp/PreDown/PostDown directives are NOT allowed.
func ValidateQuickServer(kind string, data []byte) error {
	if len(data) == 0 || len(data) > 1<<20 {
		return errors.New("tunnel config outside size limit")
	}
	base := "10.76.0.0/16"
	interfaceAddress := "10.76.0.1/16"
	if kind == "amneziawg" {
		base = "10.77.0.0/16"
		interfaceAddress = "10.77.0.1/16"
	} else if kind != "wireguard" {
		return errors.New("unsupported tunnel kind")
	}
	prefix := netip.MustParsePrefix(base)
	section := ""
	interfaceKeys := map[string]bool{}
	peerKeys := map[string]bool{}
	peers := map[string]bool{}
	count := 0
	peerCount := 0
	flushPeer := func() error {
		if section == "peer" && (!peerKeys["PublicKey"] || !peerKeys["AllowedIPs"]) {
			return errors.New("incomplete tunnel peer")
		}
		return nil
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		switch line {
		case "[Interface]":
			if section != "" {
				return errors.New("duplicate/out-of-order tunnel interface")
			}
			section = "interface"
			continue
		case "[Peer]":
			if section == "" {
				return errors.New("peer without interface")
			}
			if err := flushPeer(); err != nil {
				return err
			}
			section = "peer"
			peerKeys = map[string]bool{}
			peerCount++
			if peerCount > 65000 {
				return errors.New("too many peers")
			}
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 || section == "" {
			return errors.New("unexpected tunnel config line")
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		count++
		if count > 200000 {
			return errors.New("tunnel config too long")
		}
		if value == "" || strings.ContainsAny(value, "\r\n;#`$\\") {
			return errors.New("unsafe tunnel config value")
		}
		switch section {
		case "interface":
			if interfaceKeys[key] {
				return errors.New("duplicate interface directive")
			}
			interfaceKeys[key] = true
			switch key {
			case "Address":
				if value != interfaceAddress {
					return errors.New("unexpected interface address")
				}
			case "ListenPort":
				n, e := strconv.Atoi(value)
				if e != nil || n < 1 || n > 65535 {
					return errors.New("invalid listen port")
				}
			case "PrivateKey":
				if !quickKey(value) {
					return errors.New("invalid private key")
				}
			case "RandomTrailers", "DisableCookies":
				if kind != "amneziawg" || (key == "RandomTrailers" && value != "off") || (key == "DisableCookies" && value != "off") {
					return errors.New("invalid AWG 3.1 feature")
				}
			case "HeaderProtectionKey":
				if kind != "amneziawg" || !quickKey(value) {
					return errors.New("invalid header key")
				}
			case "Jc", "Jmin", "Jmax", "S1", "S2", "S3", "S4", "H1", "H2", "H3", "H4", "ContentPaddingAddition":
				if kind != "amneziawg" || value != awgStaticValue(key) {
					return errors.New("invalid AWG setting")
				}
			default:
				return fmt.Errorf("forbidden interface directive: %q", key)
			}
		case "peer":
			if peerKeys[key] {
				return errors.New("duplicate peer directive")
			}
			peerKeys[key] = true
			switch key {
			case "PublicKey":
				if !quickKey(value) {
					return errors.New("invalid peer key")
				}
			case "AllowedIPs":
				peer, e := netip.ParsePrefix(value)
				if e != nil || peer.Bits() != 32 || !prefix.Contains(peer.Addr()) || peer.Addr() == netip.MustParseAddr(strings.TrimSuffix(interfaceAddress, "/16")) {
					return errors.New("invalid peer address")
				}
				if peers[value] {
					return errors.New("duplicate peer address")
				}
				peers[value] = true
			default:
				return fmt.Errorf("forbidden peer directive: %q", key)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if section == "" || !interfaceKeys["Address"] || !interfaceKeys["ListenPort"] || !interfaceKeys["PrivateKey"] {
		return errors.New("missing interface data")
	}
	if kind == "amneziawg" {
		for _, key := range []string{"HeaderProtectionKey", "Jc", "Jmin", "Jmax", "S1", "S2", "S3", "S4", "H1", "H2", "H3", "H4", "ContentPaddingAddition", "RandomTrailers", "DisableCookies"} {
			if !interfaceKeys[key] {
				return errors.New("missing AWG setting")
			}
		}
	}
	return flushPeer()
}
func quickKey(text string) bool {
	b, e := base64.StdEncoding.DecodeString(text)
	return e == nil && len(b) == 32
}
func awgStaticValue(key string) string {
	switch key {
	case "Jc":
		return "5"
	case "Jmin":
		return "40"
	case "Jmax":
		return "100"
	case "S1":
		return "40"
	case "S2":
		return "40"
	case "S3", "S4":
		return "40"
	case "H1":
		return "1"
	case "H2":
		return "2"
	case "H3":
		return "3"
	case "H4":
		return "4"
	case "ContentPaddingAddition":
		return "32-128"
	}
	return ""
}

// RunQuickTunnel runs in a root-owned systemd process. The panel can write
// untrusted source configs but never passes those directly to wg-quick: this
// command snapshots, validates and stages them in /run/arcvpn, owned by root.
func RunQuickTunnel(ctx context.Context, action, kind string) error {
	if os.Geteuid() != 0 {
		return errors.New("root-only tunnel command")
	}
	name, quick := "arcwg", "/usr/bin/wg-quick"
	if kind == "amneziawg" {
		name = "arcawg"
	} else if kind != "wireguard" {
		return errors.New("unsupported tunnel kind")
	}
	if action != "up" && action != "down" {
		return errors.New("unsupported tunnel operation")
	}
	trustedDir := "/run/arcvpn"
	target := filepath.Join(trustedDir, name+".conf")
	if action == "up" {
		content, err := os.ReadFile(filepath.Join("/var/lib/arcvpn", name+".conf"))
		if err != nil {
			return err
		}
		if err := ValidateQuickServer(kind, content); err != nil {
			return err
		}
		if err := os.MkdirAll(trustedDir, 0700); err != nil {
			return err
		}
		// No following existing symlinks: write to a new root-owned tempfile then rename.
		tmp, err := os.CreateTemp(trustedDir, ".arcvpn-*.conf")
		if err != nil {
			return err
		}
		defer os.Remove(tmp.Name())
		if err = tmp.Chmod(0600); err != nil {
			tmp.Close()
			return err
		}
		if _, err = tmp.Write(content); err != nil {
			tmp.Close()
			return err
		}
		if err = tmp.Sync(); err != nil {
			tmp.Close()
			return err
		}
		if err = tmp.Close(); err != nil {
			return err
		}
		if err = os.Rename(tmp.Name(), target); err != nil {
			return err
		}
	}
	c, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	if kind == "amneziawg" {
		if err := runAmneziaUserspace(c, action, target); err != nil {
			return err
		}
	} else {
		cmd := exec.CommandContext(c, quick, action, target)
		result, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("tunnel %s failed: %w: %s", action, err, shortMessage(result))
		}
	}
	if action == "down" {
		_ = os.Remove(target)
	}
	return nil
}

func shortMessage(b []byte) string {
	if len(b) > 256 {
		b = b[:256]
	}
	return strings.ReplaceAll(string(b), "\x00", "")
}
