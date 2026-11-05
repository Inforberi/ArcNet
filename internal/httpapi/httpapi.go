package httpapi

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/arcvpn/arcvpn/internal/app"
	"github.com/arcvpn/arcvpn/internal/auth"
	"github.com/arcvpn/arcvpn/internal/logging"
	"github.com/arcvpn/arcvpn/internal/metrics"
	"github.com/arcvpn/arcvpn/internal/publicnet"
	"github.com/arcvpn/arcvpn/internal/store"
	"github.com/arcvpn/arcvpn/internal/targetcheck"
	"github.com/arcvpn/arcvpn/internal/xray"
	"github.com/arcvpn/arcvpn/web"
)

type Server struct {
	App                  *app.App
	log                  *slog.Logger
	limiter              loginLimiter
	assets               fs.FS
	publicURL            string
	logRotator           *logging.Rotator
	targetCatalogKey     ed25519.PublicKey
	targetSlots          chan struct{}
	loginSlots           chan struct{}
	targetChecker        targetcheck.Checker
	monitoring           *monitoringHub
	serviceCache         serviceSnapshot
	telemetry            *metrics.Sampler
	telemetryWriteFailed atomic.Bool
	bootstrapSlots       chan struct{} // bounded simultaneous outbound SSH installations
}
type loginAttempt struct {
	Count int
	Last  time.Time
}
type loginLimiter struct {
	sync.Mutex
	attempts map[string]loginAttempt
}

func New(a *app.App, logger *slog.Logger) *Server {
	directory := "."
	if a != nil && a.Store != nil && a.Store.DB != nil {
		directory = filepath.Dir(a.Store.DB.Path())
	}
	return &Server{App: a, log: logger, limiter: loginLimiter{attempts: make(map[string]loginAttempt)}, bootstrapSlots: make(chan struct{}, 2), targetSlots: make(chan struct{}, 2), loginSlots: make(chan struct{}, 1), targetChecker: targetcheck.Checker{Guard: publicnet.New()}, monitoring: newMonitoringHub(), telemetry: metrics.NewSampler(directory)}
}

// WithLogRotator enables per-application log budgeting from the web panel.
func (s *Server) WithLogRotator(r *logging.Rotator) { s.logRotator = r }

// WithWebRoot loads a React bundle from disk for development and browser tests.
// Production binaries embed the same assets and do not require --web-root.
func (s *Server) WithWebRoot(path string) error {
	if path == "" {
		return nil
	}
	root := os.DirFS(path)
	if err := validateReactAssets(root); err != nil {
		return err
	}
	s.assets = root
	return nil
}

// WithEmbeddedFrontend loads the Vite output compiled into the release binary.
func (s *Server) WithEmbeddedFrontend() error {
	assets, err := web.ReactAssets()
	if err != nil {
		return err
	}
	if err := validateReactAssets(assets); err != nil {
		return err
	}
	s.assets = assets
	return nil
}

// CheckEmbeddedFrontend is a preflight command usable before installation
// changes any system state. It rejects development binaries.
func CheckEmbeddedFrontend() error {
	assets, err := web.ReactAssets()
	if err != nil {
		return err
	}
	return validateReactAssets(assets)
}

func validateReactAssets(root fs.FS) error {
	index, err := fs.ReadFile(root, "index.html")
	if err != nil {
		return fmt.Errorf("read React index: %w", err)
	}
	if !strings.Contains(string(index), `id="root"`) {
		return errors.New("invalid React bundle: root element is missing")
	}
	for _, entry := range []struct{ name, pattern string }{
		{"JavaScript bundle", `(?i)(?:src=["'])(/assets/[a-z0-9_.-]+\.js)["']`},
		{"CSS bundle", `(?i)(?:href=["'])(/assets/[a-z0-9_.-]+\.css)["']`},
	} {
		matches := regexp.MustCompile(entry.pattern).FindSubmatch(index)
		if len(matches) < 2 {
			return fmt.Errorf("invalid React bundle: %s reference is missing", entry.name)
		}
		assetPath := strings.TrimPrefix(string(matches[1]), "/")
		stat, err := fs.Stat(root, assetPath)
		if err != nil || stat.IsDir() || stat.Size() == 0 {
			return fmt.Errorf("invalid React bundle: %s missing or empty: %s", entry.name, assetPath)
		}
	}
	return fs.WalkDir(root, ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("React bundle must not contain symlinks: %s", name)
		}
		return nil
	})
}

// WithPublicURL fixes subscription URL generation to the configured origin,
// preventing accidental Host-header poisoning through proxy configuration.
func (s *Server) WithPublicURL(raw string) error {
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("public URL must be an HTTPS origin without path")
	}
	s.publicURL = strings.TrimRight(raw, "/")
	return nil
}

func (s *Server) Routes() http.Handler {
	if err := s.App.Store.MarkInterruptedBootstraps(context.Background()); err != nil {
		s.log.Error("bootstrap recovery failed", "error", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	mux.HandleFunc("POST /api/login", s.login)
	mux.HandleFunc("GET /api/session", s.mustAuth(s.session, false))
	mux.HandleFunc("GET /api/security/mfa", s.mustAuth(s.mfaStatus, false))
	mux.HandleFunc("POST /api/security/mfa/setup", s.mustAuth(s.mfaSetup, true))
	mux.HandleFunc("POST /api/security/mfa/confirm", s.mustAuth(s.mfaConfirm, true))
	mux.HandleFunc("POST /api/security/mfa/disable", s.mustAuth(s.mfaDisable, true))
	mux.HandleFunc("POST /api/logout", s.mustAuth(s.logout, true))
	mux.HandleFunc("GET /api/servers/{id}/telemetry", s.mustAuth(s.serverTelemetry, false))
	mux.HandleFunc("POST /api/nodes/telemetry", s.nodeTelemetry)
	mux.HandleFunc("GET /api/monitoring/events", s.mustAuth(s.monitoringStream, false))
	mux.HandleFunc("GET /api/settings/monitoring", s.mustAuth(s.monitoringSettings, false))
	mux.HandleFunc("PUT /api/settings/monitoring", s.mustAuth(s.monitoringSave, true))
	mux.HandleFunc("GET /api/overview", s.mustAuth(s.overview, false))
	mux.HandleFunc("POST /api/nodes/ssh/inspect", s.mustAuth(s.nodeSSHInspect, true))
	mux.HandleFunc("POST /api/nodes/ssh/preflight", s.mustAuth(s.nodeSSHPreflight, true))
	mux.HandleFunc("POST /api/nodes/ssh/install", s.mustAuth(s.nodeSSHInstall, true))
	mux.HandleFunc("POST /api/nodes/{id}/ssh/upgrade-plan", s.mustAuth(s.nodeSSHUpgradePlan, true))
	mux.HandleFunc("POST /api/nodes/{id}/ssh/upgrade", s.mustAuth(s.nodeSSHUpgrade, true))
	mux.HandleFunc("POST /api/nodes/{id}/ssh/remove-plan", s.mustAuth(s.nodeSSHRemovePlan, true))
	mux.HandleFunc("POST /api/nodes/{id}/ssh/remove", s.mustAuth(s.nodeSSHRemove, true))
	mux.HandleFunc("GET /api/nodes/ssh/operations", s.mustAuth(s.nodeSSHOperations, false))
	mux.HandleFunc("GET /api/nodes/ssh/operations/{id}", s.mustAuth(s.nodeSSHOperation, false))
	mux.HandleFunc("GET /api/routes", s.mustAuth(s.routeList, false))
	mux.HandleFunc("POST /api/routes/preview", s.mustAuth(s.routePreview, true))
	mux.HandleFunc("POST /api/routes", s.mustAuth(s.routeCreate, true))
	mux.HandleFunc("GET /api/routes/{id}", s.mustAuth(s.routeGet, false))
	mux.HandleFunc("PUT /api/routes/{id}", s.mustAuth(s.routeUpdate, true))
	mux.HandleFunc("DELETE /api/routes/{id}", s.mustAuth(s.routeDelete, true))
	mux.HandleFunc("POST /api/routes/{id}/simulate", s.mustAuth(s.routeSimulate, true))
	mux.HandleFunc("GET /api/routing-profiles", s.mustAuth(s.routingProfiles, false))
	mux.HandleFunc("POST /api/routing-profiles", s.mustAuth(s.routingProfileSave, true))
	mux.HandleFunc("PUT /api/routing-profiles/{id}", s.mustAuth(s.routingProfileSave, true))
	mux.HandleFunc("DELETE /api/routing-profiles/{id}", s.mustAuth(s.routingProfileDelete, true))
	mux.HandleFunc("POST /api/routing-profiles/check", s.mustAuth(s.routingProfileCheck, true))
	mux.HandleFunc("POST /api/routing-profiles/import", s.mustAuth(s.routingProfileImport, true))
	mux.HandleFunc("POST /api/routing-profiles/list-preview", s.mustAuth(s.routingListPreview, true))
	mux.HandleFunc("GET /api/routing-profiles/{id}/export", s.mustAuth(s.routingProfileExport, false))

	mux.HandleFunc("GET /api/nodes", s.mustAuth(s.nodeList, false))
	mux.HandleFunc("POST /api/nodes/{id}/deploy", s.mustAuth(s.nodeDeploy, true))
	mux.HandleFunc("POST /api/nodes/{id}/drain", s.mustAuth(s.nodeDrain, true))
	mux.HandleFunc("POST /api/nodes/{id}/probe/grant", s.mustAuth(s.nodeProbeGrant, true))
	mux.HandleFunc("POST /api/nodes/probe/report", s.nodeProbeReport)
	mux.HandleFunc("GET /api/nodes/{id}/links", s.mustAuth(s.nodeDirectLinks, false))
	mux.HandleFunc("GET /api/nodes/{id}/operations", s.mustAuth(s.nodeOperationList, false))
	mux.HandleFunc("POST /api/nodes/{id}/retry", s.mustAuth(s.nodeRetry, true))
	mux.HandleFunc("POST /api/nodes/{id}/cancel", s.mustAuth(s.nodeCancel, true))
	mux.HandleFunc("GET /api/nodes/poll", s.nodePoll)
	mux.HandleFunc("GET /api/targets/catalog", s.mustAuth(s.targetCatalog, false))
	mux.HandleFunc("POST /api/targets/catalog", s.mustAuth(s.targetCatalogImport, true))
	mux.HandleFunc("GET /api/targets/nodes", s.mustAuth(s.targetNodes, false))
	mux.HandleFunc("POST /api/targets/check", s.mustAuth(s.targetCheckStart, true))
	mux.HandleFunc("GET /api/targets/checks/{id}", s.mustAuth(s.targetCheckGet, false))
	mux.HandleFunc("GET /api/nodes/target-checks/poll", s.nodeTargetPoll)
	mux.HandleFunc("POST /api/nodes/target-checks/report", s.nodeTargetReport)

	mux.HandleFunc("POST /api/nodes/ack", s.nodeAck)
	mux.HandleFunc("POST /api/nodes/rotate/begin", s.nodeRotateBegin)
	mux.HandleFunc("POST /api/nodes/rotate/confirm", s.nodeRotateConfirm)
	mux.HandleFunc("POST /api/nodes/enroll", s.mustAuth(s.nodeEnroll, true))
	mux.HandleFunc("DELETE /api/nodes/{id}", s.mustAuth(s.nodeDelete, true))
	mux.HandleFunc("POST /api/nodes/register", s.nodeRegister)
	mux.HandleFunc("POST /api/nodes/heartbeat", s.nodeHeartbeat)
	mux.HandleFunc("GET /node/binary", s.nodeBinary)
	mux.HandleFunc("GET /node-install.sh", func(w http.ResponseWriter, r *http.Request) {
		data, err := fs.ReadFile(web.Assets, "node-install.sh")
		if err != nil {
			http.Error(w, "installer unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/x-shellscript; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(data)
	})
	mux.HandleFunc("GET /api/users/usage", s.mustAuth(s.userUsage, false))
	mux.HandleFunc("GET /api/users/{id}/devices", s.mustAuth(s.userDevices, false))
	mux.HandleFunc("POST /api/users/{id}/devices", s.mustAuth(s.userDeviceChange, true))
	mux.HandleFunc("PUT /api/users/{id}/devices", s.mustAuth(s.userDeviceChange, true))
	mux.HandleFunc("DELETE /api/users/{id}/devices/{device}", s.mustAuth(s.userDeviceChange, true))
	mux.HandleFunc("GET /api/users/{id}/devices/{device}/links", s.mustAuth(s.userDeviceLinks, false))
	mux.HandleFunc("GET /device/{token}", s.deviceSubscription)
	mux.HandleFunc("GET /device/{token}/wireguard", s.deviceWireGuardExport)
	mux.HandleFunc("PUT /api/users/{id}/package", s.mustAuth(s.userPackageSave, true))
	mux.HandleFunc("GET /api/users/{id}/debits", s.mustAuth(s.userDebits, false))
	mux.HandleFunc("GET /api/users", s.mustAuth(s.users, false))
	mux.HandleFunc("POST /api/users", s.mustAuth(s.usersCreate, true))
	mux.HandleFunc("PATCH /api/users/{id}", s.mustAuth(s.userPatch, true))
	mux.HandleFunc("DELETE /api/users/{id}", s.mustAuth(s.userDelete, true))
	mux.HandleFunc("GET /api/users/{id}/subscription", s.mustAuth(s.userSubscription, false))
	mux.HandleFunc("GET /api/users/{id}/export", s.mustAuth(s.userExport, false))
	mux.HandleFunc("GET /api/users/{id}/qr", s.mustAuth(s.userQRCode, false))
	mux.HandleFunc("POST /api/backup/export", s.mustAuth(s.backupExport, true))
	mux.HandleFunc("POST /api/backup/preview", s.mustAuth(s.backupPreview, true))
	mux.HandleFunc("POST /api/backup/import", s.mustAuth(s.backupImport, true))
	mux.HandleFunc("GET /api/protocols", s.mustAuth(s.protocolList, false))
	mux.HandleFunc("GET /api/protocols/health", s.mustAuth(s.protocolHealth, false))
	mux.HandleFunc("POST /api/protocols", s.mustAuth(s.protocolCreate, true))
	mux.HandleFunc("PATCH /api/protocols/{kind}", s.mustAuth(s.protocolPatch, true))
	mux.HandleFunc("DELETE /api/protocols/{kind}", s.mustAuth(s.protocolDelete, true))
	mux.HandleFunc("GET /api/users/{id}/connections", s.mustAuth(s.userConnections, false))
	mux.HandleFunc("GET /api/users/{id}/connections/{kind}/export", s.mustAuth(s.userConnectionExport, false))
	mux.HandleFunc("GET /api/vpn/defaults", s.mustAuth(s.connectionDefaults, false))
	mux.HandleFunc("GET /api/vpn", s.mustAuth(s.vpn, false))
	mux.HandleFunc("POST /api/vpn", s.mustAuth(s.vpnCreate, true))
	mux.HandleFunc("GET /api/events", s.mustAuth(s.events, false))
	mux.HandleFunc("GET /api/system/update", s.mustAuth(s.updateStatus, false))
	mux.HandleFunc("POST /api/system/update/upload", s.mustAuth(s.updateUpload, true))
	mux.HandleFunc("POST /api/system/update/apply", s.mustAuth(s.updateApply, true))
	mux.HandleFunc("POST /api/system/update/discard", s.mustAuth(s.updateDiscard, true))
	mux.HandleFunc("GET /api/settings/firewall", s.mustAuth(s.firewallStatus, false))
	mux.HandleFunc("POST /api/settings/firewall", s.mustAuth(s.firewallSave, true))
	mux.HandleFunc("GET /api/settings/logs", s.mustAuth(s.logSettings, false))
	mux.HandleFunc("POST /api/settings/logs", s.mustAuth(s.logSettingsSave, true))
	mux.HandleFunc("GET /api/settings/journal", s.mustAuth(s.journalBudgetGet, false))
	mux.HandleFunc("POST /api/settings/journal", s.mustAuth(s.journalBudgetSave, true))
	mux.HandleFunc("GET /api/settings/journal/tail", s.mustAuth(s.journalTail, false))
	mux.HandleFunc("GET /api/settings/logs/tail", s.mustAuth(s.logTail, false))
	mux.HandleFunc("POST /api/settings/logs/clear", s.mustAuth(s.logClear, true))
	mux.HandleFunc("GET /sub/{token}", s.subscription)
	assets := s.assets
	if assets == nil {
		assets = web.Assets
	}
	fileServer := http.FileServerFS(assets)
	mux.Handle("GET /", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/sub/") || strings.HasPrefix(r.URL.Path, "/device/") {
			http.NotFound(w, r)
			return
		}
		if r.URL.Path == "/" {
			serveSPAIndex(w, assets)
			return
		}
		if strings.Contains(r.URL.Path, "..") {
			http.NotFound(w, r)
			return
		}
		if _, e := fs.Stat(assets, strings.TrimPrefix(r.URL.Path, "/")); e == nil {
			fileServer.ServeHTTP(w, r)
			return
		}
		if !strings.Contains(strings.TrimPrefix(r.URL.Path, "/"), ".") {
			serveSPAIndex(w, assets)
			return
		}
		http.NotFound(w, r)
	}))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; script-src 'self'; img-src 'self' data:; connect-src 'self'; object-src 'none'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		if r.TLS != nil {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000")
		}
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/sub/") || strings.HasPrefix(r.URL.Path, "/device/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		defer func() {
			if val := recover(); val != nil {
				s.log.Error("recovered http panic", "route", r.Pattern, "type", fmt.Sprintf("%T", val))
				http.Error(w, "internal error", http.StatusInternalServerError)
			}
		}()
		mux.ServeHTTP(w, r)
	})
}

type failure struct {
	Error string `json:"error"`
}

func respond(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, msg string) { respond(w, status, failure{msg}) }
func decode(w http.ResponseWriter, r *http.Request, out any) bool {
	limit := int64(16 * 1024)
	if strings.HasPrefix(r.URL.Path, "/api/nodes/ssh/") {
		limit = 64 * 1024
	}
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		fail(w, 400, "Неверный формат запроса")
		return false
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		fail(w, 400, "Дополнительные или некорректные данные в запросе")
		return false
	}
	return true
}
func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, e := url.Parse(origin)
	if e != nil {
		return false
	}
	expectedScheme := "http"
	if r.TLS != nil {
		expectedScheme = "https"
	}
	return u.Host == r.Host && u.Scheme == expectedScheme
}
func (s *Server) throttle(ip string) bool {
	s.limiter.Lock()
	defer s.limiter.Unlock()
	now := time.Now()
	if len(s.limiter.attempts) > 4096 {
		for k, v := range s.limiter.attempts {
			if now.Sub(v.Last) > 10*time.Minute {
				delete(s.limiter.attempts, k)
			}
		}
		if len(s.limiter.attempts) > 4096 {
			return false
		}
	}
	v := s.limiter.attempts[ip]
	if now.Sub(v.Last) > time.Minute {
		v.Count = 0
	}
	v.Count++
	v.Last = now
	s.limiter.attempts[ip] = v
	return v.Count <= 8
}

// Password work is bounded independently of IP rate limits, protecting small
// hosts from concurrent KDF allocations without weakening the password hash.
func (s *Server) verifyLoginPassword(password, stored string) (bool, bool) {
	select {
	case s.loginSlots <- struct{}{}:
		defer func() { <-s.loginSlots }()
		return auth.VerifyPassword(password, stored), false
	default:
		return false, true
	}
}
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(r) {
		fail(w, 403, "Forbidden origin")
		return
	}
	addr, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		addr = r.RemoteAddr
	}
	if !s.throttle(addr) {
		fail(w, 429, "Слишком много попыток. Повторите позднее")
		return
	}
	var input struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if !decode(w, r, &input) {
		return
	}
	if len(input.Username) > 128 || len(input.Password) > 256 {
		fail(w, 401, "Неверные учётные данные")
		return
	}
	stored, err := s.App.Store.AdminHash(r.Context(), input.Username)
	if err != nil {
		s.log.Error("login storage error", "error", err)
		fail(w, 500, "Internal error")
		return
	}
	verified, busy := s.verifyLoginPassword(input.Password, stored)
	if busy {
		w.Header().Set("Retry-After", "1")
		fail(w, 429, "Проверка входа уже выполняется. Повторите через секунду")
		return
	}
	if !verified {
		fail(w, 401, "Неверные учётные данные")
		return
	}
	enabled, err := s.App.Store.MFAStatus(r.Context())
	if err != nil {
		fail(w, 500, "Internal error")
		return
	}
	if enabled {
		accepted, checkErr := s.App.Store.CheckLoginMFA(r.Context(), input.Code, time.Now())
		if checkErr != nil {
			fail(w, 500, "Internal error")
			return
		}
		if !accepted {
			fail(w, 401, "Укажи действующий код 2FA")
			return
		}
	}
	token, err := auth.Secret(32)
	if err != nil {
		fail(w, 500, "Internal error")
		return
	}
	csrf, err := auth.Secret(32)
	if err != nil {
		fail(w, 500, "Internal error")
		return
	}
	if err = s.App.Store.AddSession(r.Context(), auth.HashToken(token), csrf, time.Now().Add(12*time.Hour).Unix()); err != nil {
		fail(w, 500, "Internal error")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "arcvpn_session", Value: token, Path: "/", Secure: r.TLS != nil, HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: 43200})
	s.App.Store.Event(r.Context(), "admin.login", "login successful")
	respond(w, 200, map[string]any{"ok": true, "csrf": csrf})
}
func (s *Server) mustAuth(next func(http.ResponseWriter, *http.Request), mutating bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		c, err := r.Cookie("arcvpn_session")
		if err != nil || len(c.Value) > 256 {
			fail(w, 401, "Требуется вход")
			return
		}
		csrf, valid, err := s.App.Store.Session(r.Context(), auth.HashToken(c.Value))
		if err != nil {
			fail(w, 500, "Internal error")
			return
		}
		if !valid {
			fail(w, 401, "Сессия истекла")
			return
		}
		if mutating && (r.Header.Get("X-CSRF-Token") != csrf || !sameOrigin(r)) {
			fail(w, 403, "CSRF check failed")
			return
		}
		next(w, r)
	}
}
func (s *Server) session(w http.ResponseWriter, r *http.Request) {
	c, _ := r.Cookie("arcvpn_session")
	csrf, _, _ := s.App.Store.Session(r.Context(), auth.HashToken(c.Value))
	username, err := s.App.Store.AdminUsername(r.Context())
	if err != nil {
		fail(w, 500, "Internal error")
		return
	}
	respond(w, 200, map[string]any{"authenticated": true, "csrf": csrf, "username": username, "mfa_enabled": func() bool { enabled, _ := s.App.Store.MFAStatus(r.Context()); return enabled }()})
}
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	c, _ := r.Cookie("arcvpn_session")
	_ = s.App.Store.DropSession(r.Context(), auth.HashToken(c.Value))
	http.SetCookie(w, &http.Cookie{Name: "arcvpn_session", Path: "/", MaxAge: -1, HttpOnly: true, Secure: r.TLS != nil, SameSite: http.SameSiteStrictMode})
	respond(w, 200, map[string]bool{"ok": true})
}
func (s *Server) overview(w http.ResponseWriter, r *http.Request) {
	total, active, err := s.App.Store.UserCounts(r.Context())
	if err != nil {
		fail(w, 500, "Не удалось прочитать количество пользователей")
		return
	}
	fleet, err := s.dashboardFleet(r.Context())
	if err != nil {
		fail(w, 500, "Не удалось получить состояние серверов")
		return
	}
	core := "unknown"
	if len(fleet) > 0 {
		switch fleet[0].Xray {
		case "active":
			core = "online"
		case "inactive", "failed":
			core = "stopped"
		case "not-configured":
			core = "not-configured"
		case "unverified":
			core = "unverified"
		}
	}
	mode := "standalone"
	if len(fleet) > 1 {
		mode = "standalone + telemetry"
	}
	sample := s.telemetry.Sample()
	respond(w, 200, map[string]any{"system": sample, "users": total, "enabled_users": active, "xray": core, "nodes": len(fleet), "mode": mode, "servers": fleet, "sampled_at": time.Now().Unix()})
}
func (s *Server) users(w http.ResponseWriter, r *http.Request) {
	users, err := s.App.Store.ListUsers(r.Context())
	if err != nil {
		fail(w, 500, "Internal error")
		return
	}
	state, err := s.App.Store.DeviceState(r.Context())
	if err != nil {
		handleErr(w, err)
		return
	}
	managed := map[int64]bool{}
	for _, record := range state {
		managed[record.UserID] = true
	}
	for i := range users {
		users[i].DevicesManaged = managed[users[i].ID]
	}
	respond(w, 200, map[string]any{"items": users})
}
func (s *Server) usersCreate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name string `json:"name"`
	}
	if !decode(w, r, &in) {
		return
	}
	u, err := s.App.AddUser(r.Context(), in.Name)
	if err != nil {
		if err.Error() == "configure VPN first" || err.Error() == "name must contain 1–60 characters" {
			fail(w, 400, err.Error())
			return
		}
		fail(w, 500, "Не удалось создать пользователя. Проверь состояние Xray и журнал панели")
		return
	}
	respond(w, 201, u)
}
func parseID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, e := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if e != nil || id <= 0 {
		fail(w, 400, "Неверный ID")
		return 0, false
	}
	return id, true
}
func (s *Server) userPatch(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var in struct {
		Enabled *bool `json:"enabled"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Enabled == nil {
		fail(w, 400, "Укажи enabled")
		return
	}
	if err := s.App.ToggleUser(r.Context(), id, *in.Enabled); err != nil {
		handleErr(w, err)
		return
	}
	respond(w, 200, map[string]bool{"ok": true})
}
func (s *Server) userDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if err := s.App.DeleteUser(r.Context(), id); err != nil {
		handleErr(w, err)
		return
	}
	respond(w, 200, map[string]bool{"ok": true})
}
func (s *Server) userSubscription(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	user, err := s.App.Store.ByID(r.Context(), id)
	if err != nil {
		handleErr(w, err)
		return
	}
	cfg, err := s.App.Store.ReadReality(r.Context())
	if err != nil {
		handleErr(w, err)
		return
	}
	base := s.origin(r)
	if base == "" {
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		base = fmt.Sprintf("%s://%s", scheme, r.Host)
	}
	links, linkErr := s.App.SubscriptionLinks(r.Context(), user)
	if linkErr != nil {
		handleErr(w, linkErr)
		return
	}
	respond(w, 200, map[string]any{"url": base + "/sub/" + user.Subscription, "link": xray.Link(cfg, user), "links": links})
}
func (s *Server) vpn(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.App.Store.ReadReality(r.Context())
	if errors.Is(err, store.ErrNotFound) {
		respond(w, 200, map[string]any{"configured": false})
		return
	}
	if err != nil {
		handleErr(w, err)
		return
	}
	respond(w, 200, map[string]any{"configured": true, "address": cfg.Address, "port": cfg.Port, "target": cfg.Target, "sni": cfg.SNI, "public_key": cfg.Public, "short_id": cfg.ShortID})
}
func (s *Server) vpnCreate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Address string `json:"address"`
		Target  string `json:"target"`
		Port    int    `json:"port"`
	}
	if !decode(w, r, &in) {
		return
	}
	if err := s.App.Setup(r.Context(), in.Address, in.Target, in.Port); err != nil {
		// Raw Xray diagnostics may contain credentials; never forward them to a browser.
		fail(w, 400, "Не удалось настроить VPN. Проверь IP, SNI, порт и журнал панели")
		return
	}
	respond(w, 201, map[string]bool{"ok": true})
}
func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	events, err := s.App.Store.Events(r.Context())
	if err != nil {
		handleErr(w, err)
		return
	}
	respond(w, 200, map[string]any{"items": events})
}
func (s *Server) subscription(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	if len(token) != 43 {
		http.NotFound(w, r)
		return
	}
	user, err := s.App.Store.VerifyUserToken(r.Context(), token)
	if err != nil || !user.Enabled {
		http.NotFound(w, r)
		return
	}
	links, err := s.App.SubscriptionLinks(r.Context(), user)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	s.subscriptionUsage(w, r, user.ID)
	_, _ = w.Write([]byte(strings.Join(links, "\n") + "\n"))
}
func handleErr(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrUsageUnsupported) {
		fail(w, 409, "Для этого подключения пока нет полного учёта трафика. Сначала снимите конечные пакеты или используйте локальный Xray.")
		return
	}
	if errors.Is(err, store.ErrNotFound) {
		fail(w, 404, "Не найдено")
		return
	}
	if errors.Is(err, store.ErrConflict) {
		fail(w, 409, "Уже существует")
		return
	}
	// Unexpected filesystem, database and Xray errors can contain key material.
	fail(w, 500, "Операция не выполнена. Проверь журнал панели")
}

func serveSPAIndex(w http.ResponseWriter, assets fs.FS) {
	data, err := fs.ReadFile(assets, "index.html")
	if err != nil {
		http.Error(w, "UI not installed", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(data)
}
