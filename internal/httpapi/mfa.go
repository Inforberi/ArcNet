package httpapi

import (
	"encoding/base64"
	"github.com/arcvpn/arcvpn/internal/qrcode"
	"net"
	"net/http"
	"time"

	"github.com/arcvpn/arcvpn/internal/auth"
	"github.com/arcvpn/arcvpn/internal/mfa"
)

func (s *Server) mfaStatus(w http.ResponseWriter, r *http.Request) {
	enabled, err := s.App.Store.MFAStatus(r.Context())
	if err != nil {
		handleErr(w, err)
		return
	}
	respond(w, 200, map[string]bool{"enabled": enabled})
}
func (s *Server) reauthenticate(r *http.Request, password string) bool {
	username, err := s.App.Store.AdminUsername(r.Context())
	if err != nil {
		return false
	}
	stored, err := s.App.Store.AdminHash(r.Context(), username)
	return err == nil && auth.VerifyPassword(password, stored)
}

// mfaThrottle bounds guesses on authenticated MFA configuration endpoints.
func (s *Server) mfaThrottle(r *http.Request) bool {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}
	return s.throttle("mfa:" + ip)
}
func (s *Server) mfaSetup(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Password string `json:"password"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !s.mfaThrottle(r) {
		fail(w, 429, "Слишком много попыток")
		return
	}
	if !s.reauthenticate(r, in.Password) {
		fail(w, 403, "Неверный пароль")
		return
	}
	secret, err := mfa.NewSecret()
	if err != nil {
		handleErr(w, err)
		return
	}
	if err = s.App.Store.BeginMFA(r.Context(), secret, time.Now().Add(10*time.Minute)); err != nil {
		fail(w, 409, "2FA уже включена")
		return
	}
	username, _ := s.App.Store.AdminUsername(r.Context())
	uri := mfa.ProvisioningURI(secret, username)
	svg, err := qrcode.SVG(uri)
	if err != nil {
		handleErr(w, err)
		return
	}
	s.App.Store.Event(r.Context(), "admin.mfa.setup", "TOTP enrollment initiated")
	respond(w, 200, map[string]string{"secret": secret, "uri": uri, "qr_svg_base64": base64.StdEncoding.EncodeToString(svg), "expires_in": "600"})
}
func (s *Server) mfaConfirm(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !s.mfaThrottle(r) {
		fail(w, 429, "Слишком много попыток")
		return
	}
	if !s.reauthenticate(r, in.Password) {
		fail(w, 403, "Неверный пароль")
		return
	}
	if err := s.App.Store.ConfirmMFA(r.Context(), in.Code, time.Now()); err != nil {
		fail(w, 422, "Неверный или просроченный одноразовый код")
		return
	}
	s.App.Store.Event(r.Context(), "admin.mfa.enabled", "TOTP activated; sessions revoked")
	// Session cookies become invalid in the database; expire the cookie here too.
	http.SetCookie(w, &http.Cookie{Name: "arcvpn_session", Path: "/", MaxAge: -1, HttpOnly: true, Secure: r.TLS != nil, SameSite: http.SameSiteStrictMode})
	respond(w, 200, map[string]bool{"ok": true, "relogin_required": true})
}
func (s *Server) mfaDisable(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !s.mfaThrottle(r) {
		fail(w, 429, "Слишком много попыток")
		return
	}
	if !s.reauthenticate(r, in.Password) {
		fail(w, 403, "Неверный пароль")
		return
	}
	if err := s.App.Store.DisableMFA(r.Context(), in.Code, time.Now()); err != nil {
		fail(w, 422, "Неверный или повторный код 2FA")
		return
	}
	s.App.Store.Event(r.Context(), "admin.mfa.disabled", "TOTP disabled; sessions revoked")
	http.SetCookie(w, &http.Cookie{Name: "arcvpn_session", Path: "/", MaxAge: -1, HttpOnly: true, Secure: r.TLS != nil, SameSite: http.SameSiteStrictMode})
	respond(w, 200, map[string]bool{"ok": true, "relogin_required": true})
}
