package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/arcvpn/arcvpn/internal/app"
	"github.com/arcvpn/arcvpn/internal/auth"
	"github.com/arcvpn/arcvpn/internal/db"
	"github.com/arcvpn/arcvpn/internal/mfa"
	"github.com/arcvpn/arcvpn/internal/store"
)

func TestHTTPMFASetupLoginAndRecovery(t *testing.T) {
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
	password := "strong-administrator-password-123"
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	if err = st.CreateAdmin(ctx, "admin", hash); err != nil {
		t.Fatal(err)
	}
	router := New(&app.App{Store: st}, slog.Default()).Routes()
	request := func(path string, payload any, csrf string, cookie *http.Cookie) *httptest.ResponseRecorder {
		data, _ := json.Marshal(payload)
		r := httptest.NewRequest("POST", "http://example.test"+path, bytes.NewReader(data))
		r.RemoteAddr = "127.0.0.1:12345"
		if csrf != "" {
			r.Header.Set("X-CSRF-Token", csrf)
		}
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	login := request("/api/login", map[string]string{"username": "admin", "password": password}, "", nil)
	if login.Code != 200 {
		t.Fatalf("login: %d %s", login.Code, login.Body.String())
	}
	var reply struct {
		CSRF string `json:"csrf"`
	}
	if err = json.Unmarshal(login.Body.Bytes(), &reply); err != nil {
		t.Fatal(err)
	}
	cookie := login.Result().Cookies()[0]
	badCSRF := request("/api/security/mfa/setup", map[string]string{"password": password}, "", cookie)
	if badCSRF.Code != 403 {
		t.Fatalf("MFA setup bypassed CSRF: %d", badCSRF.Code)
	}
	setup := request("/api/security/mfa/setup", map[string]string{"password": password}, reply.CSRF, cookie)
	if setup.Code != 200 {
		t.Fatalf("MFA setup: %d %s", setup.Code, setup.Body.String())
	}
	var info struct {
		Secret string `json:"secret"`
		URI    string `json:"uri"`
		QR     string `json:"qr_svg_base64"`
	}
	if err = json.Unmarshal(setup.Body.Bytes(), &info); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(info.URI, "otpauth://totp/") || info.QR == "" {
		t.Fatal("authenticator provisioning data missing")
	}
	code, err := mfa.Code(info.Secret, time.Now().Unix()/30, 6)
	if err != nil {
		t.Fatal(err)
	}
	confirm := request("/api/security/mfa/confirm", map[string]string{"password": password, "code": code}, reply.CSRF, cookie)
	if confirm.Code != 200 {
		t.Fatalf("MFA confirmation: %d %s", confirm.Code, confirm.Body.String())
	}
	oldSession := httptest.NewRequest("GET", "http://example.test/api/session", nil)
	oldSession.AddCookie(cookie)
	r := httptest.NewRecorder()
	router.ServeHTTP(r, oldSession)
	if r.Code != 401 {
		t.Fatalf("existing session not revoked: %d", r.Code)
	}
	noCode := request("/api/login", map[string]string{"username": "admin", "password": password}, "", nil)
	if noCode.Code != 200 || !strings.Contains(noCode.Body.String(), `"mfa_required":true`) || len(noCode.Result().Cookies()) != 0 {
		t.Fatal("second factor must be requested without issuing a session")
	}
	replay := request("/api/login", map[string]string{"username": "admin", "password": password, "code": code}, "", nil)
	if replay.Code != 401 {
		t.Fatal("TOTP replay accepted")
	}
	newHash, err := auth.HashPassword("replacement-administrator-password-789")
	if err != nil {
		t.Fatal(err)
	}
	if err = st.ResetAdminPassword(ctx, newHash); err != nil {
		t.Fatal(err)
	}
	recovered := request("/api/login", map[string]string{"username": "admin", "password": "replacement-administrator-password-789"}, "", nil)
	if recovered.Code != 200 {
		t.Fatalf("CLI-style recovery did not clear MFA: %d %s", recovered.Code, recovered.Body.String())
	}
}
