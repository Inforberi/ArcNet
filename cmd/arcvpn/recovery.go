package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/arcvpn/arcvpn/internal/auth"
	"github.com/arcvpn/arcvpn/internal/nodeprotocol"
	"github.com/arcvpn/arcvpn/internal/store"
)

func resetAdministrator(ctx context.Context, st *store.Store, args []string, input io.Reader, output io.Writer) error {
	flags := flag.NewFlagSet("admin-reset", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	stdin := flags.Bool("password-stdin", false, "read new password from stdin")
	resetMFA := flags.Bool("reset-2fa", false, "explicitly remove administrator MFA")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		return errors.New("использование: admin-reset [--password-stdin] [--reset-2fa]; пароль не передаётся аргументом")
	}
	var password string
	if *stdin {
		raw, err := io.ReadAll(io.LimitReader(input, 259))
		if err != nil {
			return errors.New("не удалось прочитать пароль")
		}
		password = strings.TrimSuffix(strings.TrimSuffix(string(raw), "\n"), "\r")
		if len(raw) > 258 || len(password) < 16 || len(password) > 256 || strings.ContainsAny(password, "\x00\r\n") {
			return errors.New("пароль должен содержать 16–256 байт в одной строке")
		}
	} else {
		var err error
		password, err = auth.Secret(24)
		if err != nil {
			return err
		}
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	if err = st.RecoverAdmin(ctx, hash, *resetMFA); err != nil {
		return err
	}
	// Only an explicit local recovery command prints a generated credential.
	// A supplied password is never echoed, audited or placed in process argv.
	if !*stdin {
		fmt.Fprintln(output, "Новый пароль администратора:", password)
	}
	fmt.Fprintln(output, "Пароль изменён. Все административные сессии завершены.")
	if *resetMFA {
		fmt.Fprintln(output, "2FA сброшена. Настройте её заново после входа.")
	} else {
		fmt.Fprintln(output, "2FA сохранена. При утрате второго фактора нужен явный --reset-2fa.")
	}
	return nil
}

type panelRuntime struct {
	Available     bool   `json:"available"`
	Listen        string `json:"listen,omitempty"`
	PublicURL     string `json:"public_url,omitempty"`
	TLSConfigured bool   `json:"tls_configured"`
}

func parsePanelArguments(raw []byte) (panelRuntime, error) {
	if len(raw) == 0 || len(raw) > 16384 {
		return panelRuntime{}, errors.New("invalid panel command line")
	}
	args := strings.Split(strings.TrimSuffix(string(raw), "\x00"), "\x00")
	if len(args) < 2 || (filepath.Base(args[0]) != "arcvpn-core" && filepath.Base(args[0]) != "arcvpn") || args[1] != "serve" {
		return panelRuntime{}, errors.New("not an ArcVPN panel process")
	}
	flags := flag.NewFlagSet("serve-inspect", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	listen := flags.String("listen", "", "")
	origin := flags.String("public-url", "", "")
	certificate := flags.String("tls-cert", "", "")
	key := flags.String("tls-key", "", "")
	// Consume values of known non-display options so a flag-looking value never
	// becomes a fabricated public URL or listener. Unknown versions are unknown.
	for _, name := range []string{"xray", "xray-config", "xray-service", "control-socket", "web-root", "sni-catalog-key"} {
		flags.String(name, "", "")
	}
	flags.Bool("legacy-ui", false, "")
	if err := flags.Parse(args[2:]); err != nil || flags.NArg() != 0 {
		return panelRuntime{}, errors.New("unsupported panel arguments")
	}
	result := panelRuntime{Available: true, TLSConfigured: *certificate != "" && *key != ""}
	if *listen != "" {
		host, port, err := net.SplitHostPort(*listen)
		if err != nil {
			return panelRuntime{}, errors.New("invalid listen address")
		}
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 || (host != "" && host != "localhost" && net.ParseIP(host) == nil) {
			return panelRuntime{}, errors.New("invalid listen address")
		}
		result.Listen = *listen
	}
	if *origin != "" {
		value, err := nodeprotocol.ValidateHTTPSOrigin(*origin)
		if err != nil {
			return panelRuntime{}, errors.New("invalid public URL")
		}
		result.PublicURL = value
	}

	return result, nil
}
func readPanelRuntime(pid int) (panelRuntime, error) {
	if pid < 1 {
		return panelRuntime{}, nil
	}
	// /proc is read only on the local Linux host. Do not execute or print the
	// process command line; only the validated allowlisted projection is emitted.
	file, err := os.Open(filepath.Join("/proc", strconv.Itoa(pid), "cmdline"))
	if err != nil {
		return panelRuntime{}, err
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, 16385))
	if err != nil {
		return panelRuntime{}, err
	}
	return parsePanelArguments(raw)
}
func showLocalSettings(ctx context.Context, st *store.Store, args []string, output io.Writer) error {
	flags := flag.NewFlagSet("settings", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asJSON := flags.Bool("json", false, "print safe JSON")
	pid := flags.Int("panel-pid", 0, "local running panel process ID")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || *pid < 0 {
		return errors.New("использование: settings [--json] [--panel-pid PID]")
	}
	settings, err := st.RecoverySettings(ctx)
	if err != nil {
		return err
	}
	runtime, readErr := readPanelRuntime(*pid)
	if readErr != nil {
		runtime = panelRuntime{}
	}
	if *asJSON {
		return json.NewEncoder(output).Encode(struct {
			Settings store.RecoverySettings `json:"settings"`
			Panel    panelRuntime           `json:"panel"`
		}{settings, runtime})
	}
	fmt.Fprintf(output, "Логин администратора: %q\n2FA включена: %t\nАктивных административных сессий: %d\nЛимит файлов журнала панели: %d МиБ\n", settings.Username, settings.MFAEnabled, settings.ActiveSessions, settings.PanelLogMB)
	if runtime.Available {
		fmt.Fprintf(output, "Панель слушает: %q\nПубличный HTTPS URL: %q\nTLS задан: %t\n", runtime.Listen, runtime.PublicURL, runtime.TLSConfigured)
	} else {
		fmt.Fprintln(output, "Параметры запущенной панели недоступны; проверьте arcvpn status.")
	}
	if settings.VPN != nil {
		fmt.Fprintf(output, "Основной VPN: %q:%d, включён: %t\n", settings.VPN.Address, settings.VPN.Port, settings.VPN.Enabled)
	} else {
		fmt.Fprintln(output, "Основной VPN не настроен.")
	}
	fmt.Fprintln(output, "Пароли, ключи и токены скрыты. Существующий пароль можно только сбросить.")
	return nil
}
