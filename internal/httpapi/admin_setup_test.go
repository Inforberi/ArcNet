package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/arcvpn/arcvpn/internal/app"
	"github.com/arcvpn/arcvpn/internal/auth"
	"github.com/arcvpn/arcvpn/internal/db"
	"github.com/arcvpn/arcvpn/internal/mfa"
	"github.com/arcvpn/arcvpn/internal/store"
	"github.com/arcvpn/arcvpn/internal/xray"
)

func TestAdminRegistrationAndCredentialChange(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "panel.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	st := &store.Store{DB: database}
	ctx := context.Background()
	if err = st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	code := "private-setup-code-with-enough-entropy"
	if err = st.BeginAdminSetup(ctx, auth.HashToken(code), time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	router := New(&app.App{Store: st}, slog.Default()).Routes()
	request := func(method, path string, input any, cookie *http.Cookie, csrf, origin string) *httptest.ResponseRecorder {
		blob, _ := json.Marshal(input)
		r := httptest.NewRequest(method, "https://example.test"+path, bytes.NewReader(blob))
		r.RemoteAddr = "127.0.0.1:12345"
		if cookie != nil {
			r.AddCookie(cookie)
		}
		if csrf != "" {
			r.Header.Set("X-CSRF-Token", csrf)
		}
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	password := "owner-selected-password-1234"
	input := map[string]string{"code": code, "username": "owner", "password": password}
	if w := request("POST", "/api/setup", input, nil, "", "https://evil.test"); w.Code != 403 {
		t.Fatal("cross-origin claim", w.Code)
	}
	if w := request("POST", "/api/setup", input, nil, "", ""); w.Code != 201 || len(w.Result().Cookies()) != 0 {
		t.Fatal("claim", w.Code, w.Body.String())
	}
	if w := request("POST", "/api/setup", input, nil, "", ""); w.Code != 409 {
		t.Fatal("replay", w.Code)
	}
	login := request("POST", "/api/login", map[string]string{"username": "owner", "password": password}, nil, "", "")
	if login.Code != 200 {
		t.Fatal("login", login.Code)
	}
	var session struct {
		CSRF string `json:"csrf"`
	}
	json.Unmarshal(login.Body.Bytes(), &session)
	cookie := login.Result().Cookies()[0]
	secret, err := mfa.NewSecret()
	if err != nil {
		t.Fatal(err)
	}
	if err = st.DB.WithTx(ctx, func(tx *db.Tx) error { return tx.Exec("INSERT INTO admin_mfa(id,secret) VALUES(1,?)", secret) }); err != nil {
		t.Fatal(err)
	}
	noOTP := request("POST", "/api/login", map[string]string{"username": "owner", "password": password}, nil, "", "")
	if noOTP.Code != 200 || !strings.Contains(noOTP.Body.String(), `"mfa_required":true`) || len(noOTP.Result().Cookies()) > 0 {
		t.Fatal("MFA stage issued a session")
	}
	otp, err := mfa.Code(secret, time.Now().Unix()/30, 6)
	if err != nil {
		t.Fatal(err)
	}
	withOTP := request("POST", "/api/login", map[string]string{"username": "owner", "password": password, "code": otp}, nil, "", "")
	if withOTP.Code != 200 || len(withOTP.Result().Cookies()) != 1 {
		t.Fatal("second-factor login failed", withOTP.Code)
	}
	change := map[string]string{"username": "new-owner", "current_password": password, "password": "next-owner-password-12345"}
	if w := request("PUT", "/api/security/account", change, cookie, "", ""); w.Code != 403 {
		t.Fatal("CSRF bypass", w.Code)
	}
	if w := request("PUT", "/api/security/account", change, cookie, session.CSRF, ""); w.Code != 200 {
		t.Fatal("change", w.Code, w.Body.String())
	}
	if w := request("GET", "/api/session", nil, cookie, "", ""); w.Code != 401 {
		t.Fatal("session not revoked")
	}
	if enabled, e := st.MFAStatus(ctx); e != nil || !enabled {
		t.Fatal("credential change disabled MFA", e)
	}
}

func TestActivationFailureDoesNotExposeDiagnostics(t *testing.T) {
	w := httptest.NewRecorder()
	activationFailure(w, &xray.OperationError{Stage: "validation", Err: bytes.ErrTooLarge})
	if w.Code != 422 || !strings.Contains(w.Body.String(), "xray_validation") || strings.Contains(w.Body.String(), bytes.ErrTooLarge.Error()) {
		t.Fatal(w.Body.String())
	}
}

// Disposable browser/API fixture only; no engine or real credential is used.
func TestOnboardingBrowserFixture(t *testing.T) {
	if os.Getenv("ARCVPN_ONBOARDING_BROWSER") != "1" {
		t.Skip("opt-in browser fixture")
	}
	s, _ := firstProtocolServer(t)
	ctx := context.Background()
	if err := s.App.Store.DB.WithTx(ctx, func(tx *db.Tx) error {
		if err := tx.Exec("DELETE FROM sessions"); err != nil {
			return err
		}
		return tx.Exec("DELETE FROM admins")
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.App.Store.BeginAdminSetup(ctx, auth.HashToken("disposable-browser-setup-code-12345"), time.Now().Add(15*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := s.WithWebRoot("../../frontend/dist"); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{}, 1)
	mux := http.NewServeMux()
	mux.Handle("/", s.Routes())
	mux.HandleFunc("POST /fixture-done", func(w http.ResponseWriter, r *http.Request) {
		select {
		case done <- struct{}{}:
		default:
		}
		w.WriteHeader(204)
	})
	server := &http.Server{Addr: "0.0.0.0:18393", Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	result := make(chan error, 1)
	go func() { result <- server.ListenAndServe() }()
	defer server.Close()
	t.Log("onboarding browser fixture ready on :18393")
	select {
	case <-done:
	case err := <-result:
		t.Fatal(err)
	case <-time.After(3 * time.Minute):
		t.Fatal("browser fixture timed out")
	}
}
