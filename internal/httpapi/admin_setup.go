package httpapi

import (
	"net/http"
	"regexp"

	"github.com/arcvpn/arcvpn/internal/auth"
)

var adminNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{2,63}$`)

func (s *Server) setupStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	has, err := s.App.Store.HasAdmin(r.Context())
	if err != nil {
		handleErr(w, err)
		return
	}
	available, err := s.App.Store.AdminSetupAvailable(r.Context())
	if err != nil {
		handleErr(w, err)
		return
	}
	respond(w, 200, map[string]bool{"required": !has, "available": available})
}

func (s *Server) setupAdmin(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(r) {
		fail(w, 403, "Forbidden origin")
		return
	}
	if !s.mfaThrottle(r) {
		fail(w, 429, "Слишком много попыток. Повторите позднее")
		return
	}
	var in struct {
		Code     string `json:"code"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decode(w, r, &in) {
		return
	}
	if len(in.Code) < 24 || len(in.Code) > 128 || !adminNamePattern.MatchString(in.Username) || auth.ValidateNewPassword(in.Password) != nil {
		fail(w, 422, "Логин: 3–64 латинских символа. Пароль: от 8 Unicode-символов, до 256 байт UTF-8. Укажите код из терминала.")
		return
	}
	select {
	case s.loginSlots <- struct{}{}:
		defer func() { <-s.loginSlots }()
	default:
		fail(w, 429, "Другая проверка уже выполняется. Повторите через секунду")
		return
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		handleErr(w, err)
		return
	}
	if err = s.App.Store.CompleteAdminSetup(r.Context(), auth.HashToken(in.Code), in.Username, hash); err != nil {
		fail(w, 409, "Код недействителен или истёк, либо администратор уже создан")
		return
	}
	s.App.Store.Event(r.Context(), "admin.created", "Administrator claimed with one-time setup code")
	respond(w, 201, map[string]bool{"ok": true})
}

func (s *Server) changeCredentials(w http.ResponseWriter, r *http.Request) {
	if !s.mfaThrottle(r) {
		fail(w, 429, "Слишком много попыток")
		return
	}
	var in struct {
		Username        string `json:"username"`
		Password        string `json:"password"`
		CurrentPassword string `json:"current_password"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !adminNamePattern.MatchString(in.Username) || auth.ValidateNewPassword(in.Password) != nil || len(in.CurrentPassword) > 256 {
		fail(w, 422, "Укажите логин (3–64 символа) и новый пароль: от 8 Unicode-символов, до 256 байт UTF-8")
		return
	}
	select {
	case s.loginSlots <- struct{}{}:
		defer func() { <-s.loginSlots }()
	default:
		fail(w, 429, "Другая проверка уже выполняется")
		return
	}
	username, err := s.App.Store.AdminUsername(r.Context())
	if err != nil {
		handleErr(w, err)
		return
	}
	previous, err := s.App.Store.AdminHash(r.Context(), username)
	if err != nil {
		handleErr(w, err)
		return
	}
	if !auth.VerifyPassword(in.CurrentPassword, previous) {
		fail(w, 403, "Неверный текущий пароль")
		return
	}
	next, err := auth.HashPassword(in.Password)
	if err != nil {
		handleErr(w, err)
		return
	}
	if err = s.App.Store.ChangeAdminCredentials(r.Context(), previous, in.Username, next); err != nil {
		handleErr(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "arcvpn_session", Path: "/", MaxAge: -1, HttpOnly: true, Secure: r.TLS != nil, SameSite: http.SameSiteStrictMode})
	respond(w, 200, map[string]bool{"ok": true, "relogin_required": true})
}
