package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/arcvpn/arcvpn/internal/logging"
	"github.com/arcvpn/arcvpn/internal/nodeagent"
	"github.com/arcvpn/arcvpn/internal/nodebootstrap"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/arcvpn/arcvpn/internal/app"
	"github.com/arcvpn/arcvpn/internal/auth"
	"github.com/arcvpn/arcvpn/internal/backup"
	"github.com/arcvpn/arcvpn/internal/backupfile"
	"github.com/arcvpn/arcvpn/internal/db"
	"github.com/arcvpn/arcvpn/internal/engines"
	"github.com/arcvpn/arcvpn/internal/httpapi"
	"github.com/arcvpn/arcvpn/internal/lifecycle"
	"github.com/arcvpn/arcvpn/internal/store"
	"github.com/arcvpn/arcvpn/internal/updater"
	"github.com/arcvpn/arcvpn/internal/xray"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Ошибка:", err)
		code := 1
		var coded interface{ ExitCode() int }
		if errors.As(err, &coded) {
			code = coded.ExitCode()
		}
		os.Exit(code)
	}
}
func run() error {
	if len(os.Args) < 2 {
		help()
		return nil
	}
	switch os.Args[1] {
	case "lifecycle":
		return runLifecycle(os.Args[2:])
	case "update-keygen":
		flags := flag.NewFlagSet("update-keygen", flag.ContinueOnError)
		dir := flags.String("out-dir", "", "offline signing key directory")
		if err := flags.Parse(os.Args[2:]); err != nil {
			return err
		}
		if *dir == "" {
			return errors.New("provide --out-dir for offline signing keys")
		}
		return updater.GenerateKeyPair(*dir)
	case "update-sign":
		flags := flag.NewFlagSet("update-sign", flag.ContinueOnError)
		binary := flags.String("binary", "", "complete release binary with React")
		private := flags.String("private-key", "", "offline ed25519 signing private key")
		version := flags.String("version", "", "release version")
		output := flags.String("output", "", "signed archive path")
		var components = map[string]*string{
			"xray":         flags.String("xray", "", "optional Xray release binary"),
			"hysteria":     flags.String("hysteria", "", "optional Hysteria release binary"),
			"awg":          flags.String("awg", "", "optional AmneziaWG awg executable"),
			"awg-quick":    flags.String("awg-quick", "", "optional AmneziaWG quick executable"),
			"amneziawg-go": flags.String("amneziawg-go", "", "optional AmneziaWG userspace daemon"),
		}
		if err := flags.Parse(os.Args[2:]); err != nil {
			return err
		}
		if *binary == "" || *private == "" || *version == "" || *output == "" {
			return errors.New("--binary, --private-key, --version and --output are required")
		}
		key, err := updater.ReadPrivateKey(*private)
		if err != nil {
			return err
		}
		program, err := os.ReadFile(*binary)
		if err != nil {
			return err
		}
		extras := make(map[string][]byte)
		for name, path := range components {
			if *path == "" {
				continue
			}
			content, readErr := os.ReadFile(*path)
			if readErr != nil {
				return fmt.Errorf("read component %s: %w", name, readErr)
			}
			extras[name] = content
		}
		// The AWG userspace daemon and its tools are one compatible release set.
		// A partial replacement could leave an incompatible service installed.
		awgCount := 0
		for _, name := range []string{"awg", "awg-quick", "amneziawg-go"} {
			if _, ok := extras[name]; ok {
				awgCount++
			}
		}
		if awgCount != 0 && awgCount != 3 {
			return errors.New("AmneziaWG update requires --awg, --awg-quick and --amneziawg-go together")
		}
		archive, err := updater.MakePackageWithComponents(program, *version, key, extras)
		if err != nil {
			return err
		}
		f, err := os.OpenFile(*output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		_, err = f.Write(archive)
		closeErr := f.Close()
		if err != nil {
			return err
		}
		return closeErr
	case "update-execute":
		if os.Geteuid() != 0 {
			return errors.New("update-execute requires dedicated root-owned systemd unit")
		}
		return (updater.Installer{}).Apply(context.Background())
	case "helper":
		return runHelper()
	case "node-helper":
		return runNodeHelper()
	case "tunnel":
		if len(os.Args) != 4 {
			return errors.New("tunnel command needs up|down and protocol")
		}
		return engines.RunQuickTunnel(context.Background(), os.Args[2], os.Args[3])
	case "version":
		fmt.Println("arcvpn " + nodeagent.Version)
		return nil
	case "cascade-probe":
		return runCascadeProbe(os.Args[2:])
	case "node-probe":
		return runNodeProbe(os.Args[2:])
	case "frontend-check":
		if err := httpapi.CheckEmbeddedFrontend(); err != nil {
			return err
		}
		fmt.Println("embedded React frontend: OK")
		return nil
	case "help":
		help()
		return nil
	}
	// Node commands must not open or initialize the central controller database.
	if os.Args[1] == "node-register" || os.Args[1] == "node-agent" || os.Args[1] == "node-install" || os.Args[1] == "node-remove" || os.Args[1] == "node-rebind" {
		return runNodeCommand(os.Args[1], os.Args[2:])
	}
	dataDir := os.Getenv("ARCVPN_DATA_DIR")
	if dataDir == "" {
		dataDir = "/var/lib/arcvpn"
	}
	if os.Args[1] == "admin-reset" || os.Args[1] == "settings" || os.Args[1] == "admin-revoke-sessions" || strings.HasPrefix(os.Args[1], "runtime-") {
		if err := checkRecoveryPermissions(dataDir); err != nil {
			return err
		}
		info, err := os.Lstat(filepath.Join(dataDir, "panel.sqlite"))
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0022 != 0 {
			return errors.New("local controller database is missing or unsafe; recovery does not create a new installation")
		}
	}
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return err
	}
	database, err := db.Open(filepath.Join(dataDir, "panel.sqlite"))
	if err != nil {
		return err
	}
	defer database.Close()
	st := &store.Store{DB: database}
	ctx := context.Background()
	if os.Args[1] != "settings" && !strings.HasPrefix(os.Args[1], "runtime-") {
		if err := st.Migrate(ctx); err != nil {
			return err
		}
	}
	switch os.Args[1] {
	case "runtime-plan", "runtime-prepare", "runtime-apply":
		if len(os.Args) != 2 {
			return errors.New("runtime management accepts no extra arguments")
		}
		call, cancel := context.WithTimeout(ctx, 45*time.Second)
		defer cancel()
		if os.Args[1] == "runtime-plan" {
			plan, err := st.RuntimePlan(call)
			if err != nil {
				return err
			}
			return json.NewEncoder(os.Stdout).Encode(plan)
		}
		a := localRuntimeApp(st, dataDir)
		if os.Args[1] == "runtime-prepare" {
			return a.FinalLocalUsage(call)
		}
		return a.ApplyLocalRuntime(call)
	case "backup-create":
		flags := flag.NewFlagSet("backup-create", flag.ContinueOnError)
		directory := flags.String("directory", "/var/backups/arcvpn", "private backup directory")
		passwordPath := flags.String("password-file", "", "path to private file with backup passphrase")
		passwordStdin := flags.Bool("password-stdin", false, "read passphrase from standard input (interactive terminal menu)")
		keep := flags.Int("keep", 7, "number of encrypted archives to retain (1–30)")
		if err := flags.Parse(os.Args[2:]); err != nil {
			return err
		}
		if (*passwordPath == "") == !*passwordStdin {
			return errors.New("choose exactly one of --password-file or --password-stdin")
		}
		var pass string
		if *passwordStdin {
			b, e := io.ReadAll(io.LimitReader(os.Stdin, 1026))
			if e != nil {
				return e
			}
			pass = strings.TrimSuffix(string(b), "\n")
			if len(pass) < 12 || len(pass) > 1024 {
				return errors.New("backup password must contain 12–1024 bytes")
			}
		} else {
			var e error
			pass, e = backupfile.PasswordFile(*passwordPath)
			if e != nil {
				return e
			}
		}
		snapshot, err := (&app.App{Store: st}).ExportState(ctx)
		if err != nil {
			return err
		}
		blob, err := backup.Encrypt(snapshot, pass)
		if err != nil {
			return err
		}
		path, err := backupfile.Save(*directory, blob, *keep)
		if err != nil {
			return err
		}
		fmt.Println("Encrypted backup written:", path)
		return nil
	case "admin-reset":
		return resetAdministrator(ctx, st, os.Args[2:], os.Stdin, os.Stdout)
	case "admin-revoke-sessions":
		if len(os.Args) != 2 {
			return errors.New("admin-revoke-sessions does not accept arguments")
		}
		if err := st.RevokeAdminSessions(ctx); err != nil {
			return err
		}
		fmt.Println("Все административные сессии завершены. VPN-подключения не изменены.")
		return nil
	case "settings":
		return showLocalSettings(ctx, st, os.Args[2:], os.Stdout)
	case "admin-init":
		flagset := flag.NewFlagSet("admin-init", flag.ContinueOnError)
		username := flagset.String("username", "admin", "administrator login")
		if err := flagset.Parse(os.Args[2:]); err != nil {
			return err
		}
		if strings.TrimSpace(*username) == "" || len(*username) > 64 {
			return errors.New("invalid username")
		}
		pwd, err := auth.Secret(24)
		if err != nil {
			return err
		}
		hash, err := auth.HashPassword(pwd)
		if err != nil {
			return err
		}
		if err := st.CreateAdmin(ctx, *username, hash); err != nil {
			return err
		}
		fmt.Printf("Login: %s\nPassword: %s\nSAVE THIS PASSWORD NOW. It will not be shown again.\n", *username, pwd)
		return nil
	case "serve":
		fs := flag.NewFlagSet("serve", flag.ContinueOnError)
		addr := fs.String("listen", "127.0.0.1:8181", "listen address")
		cert := fs.String("tls-cert", "", "trusted cert PEM file")
		key := fs.String("tls-key", "", "private key PEM file")
		coreBinary := fs.String("xray", "/usr/local/bin/xray", "xray binary")
		coreConf := fs.String("xray-config", filepath.Join(dataDir, "xray.json"), "managed xray config")
		coreService := fs.String("xray-service", "arcvpn-xray.service", "xray systemd unit name")
		controlSocket := fs.String("control-socket", "", "privileged local Unix socket for service restart")
		catalogKey := fs.String("sni-catalog-key", "", "independently pinned target catalog Ed25519 public key file")
		webRoot := fs.String("web-root", "", "development React/Vite dist override (production uses embedded UI)")
		legacyUI := fs.Bool("legacy-ui", false, "developer-only compatibility UI (loopback HTTP only)")
		publicURL := fs.String("public-url", "", "public HTTPS origin for subscriptions")
		if err := fs.Parse(os.Args[2:]); err != nil {
			return err
		}
		if (*cert == "") != (*key == "") {
			return errors.New("TLS cert and key must be provided together")
		}
		if *cert == "" {
			host, _, err := splitHostPort(*addr)
			if err != nil {
				return err
			}
			if host != "127.0.0.1" && host != "::1" && host != "localhost" {
				return errors.New("HTTP may bind to loopback only; configure trusted TLS for public access")
			}
		}
		hasAdmin, err := st.HasAdmin(ctx)
		if err != nil {
			return err
		}
		if !hasAdmin {
			return errors.New("administrator is missing; run admin-init")
		}
		_, listenerPortRaw, splitErr := net.SplitHostPort(*addr)
		if splitErr != nil {
			return splitErr
		}
		listenerPort, portErr := strconv.Atoi(listenerPortRaw)
		if portErr != nil {
			return portErr
		}
		a := &app.App{
			ReservedTCPPort: listenerPort,
			RuntimeCheck:    (lifecycle.Guard{}).Check,
			Store:           st,
			Core:            xray.Controller{Binary: *coreBinary, Path: *coreConf, Service: *coreService, ControlSocket: *controlSocket},
			Engines: &engines.Manager{
				Dir: dataDir, HelperSocket: *controlSocket,
				HysteriaBinary: "/opt/arcvpn/bin/hysteria",
				WGQuickBinary:  "/usr/bin/wg-quick",
				AWGQuickBinary: "/opt/arcvpn/bin/awg-quick",
			},
		}
		budget, err := st.LogBudgetMB(ctx)
		if err != nil {
			return err
		}
		rotator, err := logging.New(filepath.Join(dataDir, "logs", "panel.log"), budget)
		if err != nil {
			return err
		}
		defer rotator.Close()
		logger := slog.New(slog.NewTextHandler(rotator, &slog.HandlerOptions{Level: slog.LevelInfo}))
		api := httpapi.New(a, logger)
		if *catalogKey != "" {
			if err := api.WithTargetCatalogKey(*catalogKey); err != nil {
				return fmt.Errorf("target catalog trust key: %w", err)
			}
		}
		api.WithLogRotator(rotator)
		if err := api.WithPublicURL(*publicURL); err != nil {
			return err
		}
		if *legacyUI && (*cert != "" || *webRoot != "") {
			return errors.New("legacy UI is allowed only on local HTTP without a React web root")
		}
		if !*legacyUI {
			if *webRoot != "" {
				if err := api.WithWebRoot(*webRoot); err != nil {
					return fmt.Errorf("frontend directory: %w", err)
				}
			} else if err := api.WithEmbeddedFrontend(); err != nil {
				return fmt.Errorf("embedded frontend: %w", err)
			}
		}
		handler := api.Routes()
		server := &http.Server{Addr: *addr, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 * 1024}
		if *cert != "" {
			server.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
		}
		exitCtx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
		defer stop()
		telemetryDone := make(chan struct{})
		go func() { defer close(telemetryDone); api.RunTelemetry(exitCtx) }()
		usageDone := make(chan struct{})
		go func() {
			defer close(usageDone)
			a.RunUsage(exitCtx, func(err error) { logger.Warn("user accounting", "error", err) })
		}()
		defer func() { stop(); <-usageDone }()
		defer func() { stop(); <-telemetryDone }()
		errs := make(chan error, 1)
		go func() {
			if *cert != "" {
				errs <- server.ListenAndServeTLS(*cert, *key)
			} else {
				errs <- server.ListenAndServe()
			}
		}()
		select {
		case err := <-errs:
			if errors.Is(err, http.ErrServerClosed) {
				return nil
			}
			return err
		case <-exitCtx.Done():
			c, cancel := context.WithTimeout(context.Background(), 12*time.Second)
			defer cancel()
			return server.Shutdown(c)
		}
	case "doctor":
		admin, err := st.HasAdmin(ctx)
		if err != nil {
			return err
		}
		fmt.Println("database: OK, admin:", admin)
		reality, err := st.ReadReality(ctx)
		if errors.Is(err, store.ErrNotFound) {
			fmt.Println("VPN: not configured")
			return nil
		}
		if err != nil {
			return err
		}
		fmt.Printf("VPN: %s:%d, target=%s (credentials hidden)\n", reality.Address, reality.Port, reality.Target)
		return nil
	default:
		help()
		return fmt.Errorf("unknown command: %s", os.Args[1])
	}
}
func splitHostPort(s string) (string, string, error) { return net.SplitHostPort(s) }
func help() {
	fmt.Println("local management protocol: lifecycle-v1")
	fmt.Println("arcvpn: serve | admin-init | admin-reset [--password-stdin] [--reset-2fa] | admin-revoke-sessions | settings [--json] [--panel-pid PID] | node-register | node-agent | node-install | node-remove | node-rebind | node-probe | cascade-probe | frontend-check | backup-create | update-keygen | update-sign | doctor | version | help")
}

// runHelper is intended ONLY for the root-owned systemd socket-activated unit.
// It accepts a single whitelisted operation. The public HTTP API never executes shell.
func runHelper() error {
	if os.Geteuid() != 0 {
		return errors.New("helper must be started as root by systemd")
	}
	account, err := user.Lookup("arcvpn")
	if err != nil {
		return err
	}
	expected, err := strconv.Atoi(account.Uid)
	if err != nil {
		return err
	}
	f := os.NewFile(3, "systemd-listen-socket")
	if f == nil {
		return errors.New("missing systemd socket")
	}
	defer f.Close()
	listener, err := net.FileListener(f)
	if err != nil {
		return err
	}
	defer listener.Close()
	unix, ok := listener.(*net.UnixListener)
	if !ok {
		return errors.New("expected Unix listener")
	}
	for {
		_ = unix.SetDeadline(time.Now().Add(45 * time.Second))
		conn, err := unix.AcceptUnix()
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				return nil
			}
			return err
		}
		func() {
			defer conn.Close()
			_ = conn.SetDeadline(time.Now().Add(16 * time.Second))
			raw, err := conn.SyscallConn()
			if err != nil {
				return
			}
			var uid uint32
			var peerErr error
			if err = raw.Control(func(fd uintptr) {
				var cred *syscall.Ucred
				cred, peerErr = syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
				if peerErr == nil {
					uid = cred.Uid
				}
			}); err != nil || peerErr != nil || uid != uint32(expected) {
				return
			}
			line, err := bufio.NewReader(io.LimitReader(conn, 32)).ReadString('\n')
			if err != nil {
				_, _ = conn.Write([]byte("error\n"))
				return
			}
			var action, unit string
			switch line {
			case "restart\n":
				action, unit = "restart", "arcvpn-xray.service"
			case "stop\n":
				action, unit = "stop", "arcvpn-xray.service"
			case "restart hysteria\n":
				action, unit = "restart", "arcvpn-hysteria.service"
			case "stop hysteria\n":
				action, unit = "stop", "arcvpn-hysteria.service"
			case "restart wireguard\n":
				action, unit = "restart", "arcvpn-wireguard.service"
			case "stop wireguard\n":
				action, unit = "stop", "arcvpn-wireguard.service"
			case "restart amneziawg\n":
				action, unit = "restart", "arcvpn-amneziawg.service"
			case "stop amneziawg\n":
				action, unit = "stop", "arcvpn-amneziawg.service"
			case "status hysteria\n":
				action, unit = "status", "arcvpn-hysteria.service"
			case "status wireguard\n":
				action, unit = "status", "arcvpn-wireguard.service"
			case "stats wireguard\n":
				ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
				defer cancel()
				report, err := engines.ReadWireGuardStats(ctx)
				if err != nil {
					_, _ = io.WriteString(conn, "error\n")
				} else {
					_ = json.NewEncoder(conn).Encode(report)
				}
				return
			case "status amneziawg\n":
				action, unit = "status", "arcvpn-amneziawg.service"
			case "journal-budget status\n":
				budget, readErr := journalBudgetRead()
				if readErr != nil {
					_, _ = conn.Write([]byte("error\n"))
				} else {
					_, _ = conn.Write([]byte(strconv.Itoa(budget) + "\n"))
				}
				return
			case "journal panel\n":
				action, unit = "journal", "arcvpn-panel.service"
			case "journal xray\n":
				action, unit = "journal", "arcvpn-xray.service"
			case "journal hysteria\n":
				action, unit = "journal", "arcvpn-hysteria.service"
			case "journal wireguard\n":
				action, unit = "journal", "arcvpn-wireguard.service"
			case "journal amneziawg\n":
				action, unit = "journal", "arcvpn-amneziawg.service"
			default:
				if strings.HasPrefix(line, "journal-budget ") {
					budget, parseErr := parseJournalBudget(line)
					if parseErr != nil || journalBudgetWrite(budget) != nil {
						_, _ = conn.Write([]byte("error\n"))
					} else {
						_, _ = conn.Write([]byte(strconv.Itoa(budget) + "\n"))
					}
					return
				}
				_, _ = conn.Write([]byte("error\n"))
				return
			}
			ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
			defer cancel()
			if action == "journal" {
				command := exec.CommandContext(ctx, "/usr/bin/journalctl", "--namespace=arcvpn", "--no-pager", "--quiet", "--output=short-iso", "-n", "80", "--unit", unit)
				outputBuffer := &boundedOutput{limit: 20 << 10}
				command.Stdout = outputBuffer
				command.Stderr = io.Discard
				readErr := command.Run()
				output := outputBuffer.data
				if readErr != nil {
					_, _ = conn.Write([]byte("error\n"))
					return
				}
				if len(output) > 20<<10 {
					output = output[len(output)-(20<<10):]
				}
				if len(output) == 0 {
					output = []byte("(журнал пуст)\n")
				}
				_, _ = conn.Write(output)
				return
			}
			if action == "status" {
				stateOutput, stateErr := exec.CommandContext(ctx, "/usr/bin/systemctl", "is-active", unit).CombinedOutput()
				state, parseErr := parseSystemdState(stateOutput, stateErr)
				if parseErr != nil {
					_, _ = conn.Write([]byte("error\n"))
				} else {
					_, _ = conn.Write([]byte(state + "\n"))
				}
				return
			}
			if action == "restart" {
				if err := (lifecycle.Guard{}).Check(); err != nil {
					_, _ = io.WriteString(conn, "error\n")
					return
				}
			}
			_, err = exec.CommandContext(ctx, "/usr/bin/systemctl", action, unit).CombinedOutput()
			if err == nil && action == "restart" {
				_, err = exec.CommandContext(ctx, "/usr/bin/systemctl", "is-active", "--quiet", unit).CombinedOutput()
			}
			if err != nil {
				_, _ = conn.Write([]byte("error\n"))
				return
			}
			_, _ = conn.Write([]byte("ok\n"))
		}()
	}
}

// A broken D-Bus connection, unknown unit or unexpected systemd reply must
// not be mistaken for a stopped service. An unsafe guessed state would make
// rollback start or stop the wrong listeners.
func parseSystemdState(output []byte, cmdErr error) (string, error) {
	switch strings.TrimSpace(string(output)) {
	case "active":
		if cmdErr == nil {
			return "active", nil
		}
	case "inactive", "failed":
		return "inactive", nil
	}
	return "", errors.New("cannot determine systemd service state")
}

// The root-operated wrapper drops privileges before opening the database.
func checkRecoveryPermissions(dir string) error {
	if os.Geteuid() == 0 {
		return errors.New("use sudo arcvpn for local management; the core must run as the arcvpn service user")
	}
	identity, err := user.Current()
	if err != nil {
		return err
	}
	if identity.Username != "arcvpn" {
		return errors.New("administrator recovery requires local arcvpn service identity")
	}
	if filepath.Clean(dir) != "/var/lib/arcvpn" && os.Getenv("ARCVPN_TEST_RECOVERY") != "1" {
		return errors.New("unexpected data directory")
	}
	return nil
}

func runNodeCommand(name string, args []string) error {
	ctx := context.Background()
	switch name {
	case "node-install":
		f := flag.NewFlagSet("node-install", flag.ContinueOnError)
		bundle := f.String("signed-bundle", "", "Ed25519 signed node release archive")
		key := f.String("public-key", "", "independently pinned Ed25519 public key")
		controller := f.String("controller", "", "trusted HTTPS controller origin")
		port := f.Int("vpn-port", 443, "VPN listener port")
		stdin := f.Bool("token-stdin", false, "read one-time enrollment token from stdin")
		if err := f.Parse(args); err != nil {
			return err
		}
		if f.NArg() != 0 || !*stdin {
			return errors.New("node-install requires --token-stdin and no positional arguments")
		}
		// Verify the independently pinned signed archive before reading the
		// one-time secret or making any change to the host.
		release, err := nodebootstrap.LoadRelease(*bundle, *key)
		if err != nil {
			return err
		}
		secret, err := readEnrollmentToken(os.Stdin)
		if err != nil {
			return err
		}
		opID, err := nodebootstrap.InstallLocal(ctx, release, *controller, secret, *port)
		if err != nil {
			return fmt.Errorf("node-install %s: %w", opID, err)
		}
		fmt.Printf("Managed node registered; installation ID: %s. VPN still requires controller deployment and independent handshake verification.\n", opID)
		return nil
	case "node-remove":
		f := flag.NewFlagSet("node-remove", flag.ContinueOnError)
		mode := f.String("mode", "", "detach keeps VPN online; purge stops and removes managed resources")
		confirm := f.String("confirm", "", "explicit confirmation phrase")
		if err := f.Parse(args); err != nil {
			return err
		}
		if f.NArg() != 0 || *confirm != "REMOVE_MANAGED_ARCVPN" {
			return errors.New("node-remove needs --confirm REMOVE_MANAGED_ARCVPN")
		}
		if os.Geteuid() != 0 {
			return errors.New("node-remove requires root")
		}
		marker, err := os.ReadFile("/etc/arcvpn-node/.arcvpn-managed")
		if err != nil {
			return err
		}
		id := strings.TrimPrefix(strings.TrimSpace(string(marker)), "detached:")
		if err := nodebootstrap.RemoveLocal(ctx, id, nodebootstrap.UninstallMode(*mode)); err != nil {
			return err
		}
		fmt.Println("Selected ArcVPN managed resources removed. Revoke old node identity in the controller.")
		return nil
	case "node-rebind":
		f := flag.NewFlagSet("node-rebind", flag.ContinueOnError)
		controller := f.String("controller", "", "new HTTPS controller origin")
		stdin := f.Bool("token-stdin", false, "read new enrollment credential from stdin")
		oldRevoked := f.Bool("old-controller-revoked", false, "confirm old controller identity was revoked")
		path := f.String("config", "/var/lib/arcvpn-node/node.json", "managed agent credential path")
		if err := f.Parse(args); err != nil {
			return err
		}
		if f.NArg() != 0 || !*stdin || !*oldRevoked {
			return errors.New("node-rebind requires --token-stdin and --old-controller-revoked")
		}
		if os.Geteuid() != 0 {
			return errors.New("node-rebind requires root to stop the existing service")
		}
		secret, err := readEnrollmentToken(os.Stdin)
		if err != nil {
			return err
		}
		// Ensure that a previously running controller agent cannot continue
		// applying commands during the authority switch.
		stop := exec.CommandContext(ctx, "systemctl", "stop", "arcvpn-node.service")
		if err := stop.Run(); err != nil {
			return errors.New("could not stop the managed agent before rebind")
		}
		defer func() {
			// Restart even when enrollment is rejected; the old credential file
			// is still intact, so a failure never silently strands the process.
			_ = exec.Command("systemctl", "start", "arcvpn-node.service").Run()
		}()
		if err := nodeagent.Rebind(ctx, *controller, secret, *path); err != nil {
			return err
		}
		start := exec.CommandContext(ctx, "systemctl", "start", "arcvpn-node.service")
		if err := start.Run(); err != nil {
			return errors.New("rebind stored new identity; start managed node-agent.service manually")
		}
		fmt.Println("Agent re-registered to the new controller. Existing Xray state was not modified.")
		return nil
	case "node-register":
		f := flag.NewFlagSet("node-register", flag.ContinueOnError)
		controller := f.String("controller", "", "HTTPS controller origin")
		tokenStdin := f.Bool("token-stdin", false, "read enrollment token from stdin (not argv)")
		output := f.String("config", "/var/lib/arcvpn-node/node.json", "agent config")
		if err := f.Parse(os.Args[2:]); err != nil {
			return err
		}
		if !*tokenStdin {
			return errors.New("node-register requires --token-stdin; passing secrets in argv is prohibited")
		}
		token, readErr := readEnrollmentToken(os.Stdin)
		if readErr != nil {
			return readErr
		}
		if err := nodeagent.Register(ctx, *controller, token, *output); err != nil {
			return err
		}
		fmt.Println("Node identity saved; enrollment token consumed")
		return nil
	case "node-agent":
		f := flag.NewFlagSet("node-agent", flag.ContinueOnError)
		path := f.String("config", "/var/lib/arcvpn-node/node.json", "agent config")
		if err := f.Parse(os.Args[2:]); err != nil {
			return err
		}
		conf, err := nodeagent.ReadConfig(*path)
		if err != nil {
			return err
		}
		runCtx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
		defer stop()
		nodeagent.Run(runCtx, conf, *path, func(err error) { fmt.Fprintln(os.Stderr, "agent communication or apply failed:", err) })
		return nil

	}
	return errors.New("unknown node command")
}

// boundedOutput accepts all subprocess bytes but retains only a fixed prefix;
// no journal line can force the root helper to allocate unbounded memory.
type boundedOutput struct {
	data  []byte
	limit int
}

func (b *boundedOutput) Write(p []byte) (int, error) {
	n := len(p)
	if len(b.data) < b.limit {
		left := b.limit - len(b.data)
		if left > len(p) {
			left = len(p)
		}
		b.data = append(b.data, p[:left]...)
	}
	return n, nil
}

// readEnrollmentToken consumes one bounded line, including interactive
// terminal input (Enter is sufficient; no Ctrl+D/EOF is required). It never
// echoes the credential or accepts extra data as a command argument.
func readEnrollmentToken(source io.Reader) (string, error) {
	line, err := bufio.NewReader(io.LimitReader(source, 130)).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", errors.New("cannot read enrollment credential")
	}
	value := strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
	if len(value) < 40 || len(value) > 128 || strings.ContainsAny(value, " \t\r\n\x00") {
		return "", errors.New("invalid enrollment credential")
	}
	return value, nil
}
