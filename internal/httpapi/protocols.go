package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/arcvpn/arcvpn/internal/protocols"
	"github.com/arcvpn/arcvpn/internal/qrcode"
	"github.com/arcvpn/arcvpn/internal/store"
	"github.com/arcvpn/arcvpn/internal/xray"
)

func (s *Server) protocolList(w http.ResponseWriter, r *http.Request) {
	profiles, err := s.App.ListProtocolProfiles(r.Context())
	if err != nil {
		handleErr(w, err)
		return
	}
	respond(w, 200, map[string]any{"items": profiles, "available_kinds": protocols.Kinds()})
}
func (s *Server) protocolCreate(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Kind    string `json:"kind"`
		Address string `json:"address"`
		Port    int    `json:"port"`
		SNI     string `json:"sni"`
		Target  string `json:"target"`
	}
	if !decode(w, r, &input) {
		return
	}
	if !protocols.IsSupported(input.Kind) {
		fail(w, 400, "Неподдерживаемый протокол")
		return
	}
	created, err := s.App.AddProtocol(r.Context(), input.Kind, input.Address, input.Port, input.SNI, input.Target)
	if err != nil {
		if errors.Is(err, store.ErrUsageUnsupported) {
			handleErr(w, err)
			return
		}
		if errors.Is(err, store.ErrConflict) {
			fail(w, 409, "Этот протокол уже настроен")
			return
		}
		if errors.Is(err, protocols.ErrInvalid) {
			fail(w, 400, "Некорректные настройки протокола")
			return
		}
		s.log.Warn("protocol activation failed", "kind", input.Kind)
		activationFailure(w, err)
		return
	}
	respond(w, 201, created)
}
func (s *Server) protocolPatch(w http.ResponseWriter, r *http.Request) {
	kind := r.PathValue("kind")
	if !protocols.IsSupported(kind) {
		fail(w, 400, "Неизвестный протокол")
		return
	}
	var input struct {
		Enabled *bool `json:"enabled"`
	}
	if !decode(w, r, &input) {
		return
	}
	if input.Enabled == nil {
		fail(w, 400, "Укажи enabled")
		return
	}
	if err := s.App.SetProtocolEnabled(r.Context(), kind, *input.Enabled); err != nil {
		handleErr(w, err)
		return
	}
	respond(w, 200, map[string]bool{"ok": true})
}
func (s *Server) protocolDelete(w http.ResponseWriter, r *http.Request) {
	kind := r.PathValue("kind")
	if !protocols.IsSupported(kind) {
		fail(w, 400, "Неизвестный протокол")
		return
	}
	if err := s.App.DeleteProtocol(r.Context(), kind); err != nil {
		if errors.Is(err, store.ErrLocalServerInUse) {
			fail(w, 409, "Последний профиль хранит адрес сервера для существующих пользователей. Оставьте его отключённым, добавьте другой протокол или сначала удалите пользователей.")
			return
		}
		if strings.Contains(err.Error(), "disable protocol") {
			fail(w, 409, "Сначала отключи протокол")
			return
		}
		handleErr(w, err)
		return
	}
	respond(w, 200, map[string]bool{"ok": true})
}

func (s *Server) userConnections(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	user, err := s.App.Store.ByID(r.Context(), id)
	if err != nil {
		handleErr(w, err)
		return
	}
	// Disabled users have no active connections; do not show inactive secrets
	// as if they could be imported and used.
	if s.App.UserAllowed(r.Context(), user.ID) != nil {
		w.Header().Set("Cache-Control", "no-store")
		respond(w, 200, map[string]any{"items": []protocols.Connection{}})
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	reality, err := s.App.Store.OptionalReality(r.Context())
	if err != nil {
		handleErr(w, err)
		return
	}
	others, err := s.App.Connections(r.Context(), id)
	if err != nil {
		handleErr(w, err)
		return
	}
	all := make([]protocols.Connection, 0, 1+len(others))
	if reality.Enabled {
		all = append(all, protocols.Connection{Kind: "vless-reality", Label: "VLESS + Reality TCP", Format: "uri", Link: xray.Link(reality, user)})
	}
	all = append(all, others...)
	respond(w, 200, map[string]any{"items": all})
}
func (s *Server) userConnectionExport(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	kind := r.PathValue("kind")
	if kind == "" || len(kind) > 40 {
		fail(w, 400, "Неверный протокол")
		return
	}
	user, err := s.App.Store.ByID(r.Context(), id)
	if err != nil {
		handleErr(w, err)
		return
	}
	if s.App.UserAllowed(r.Context(), user.ID) != nil {
		fail(w, 409, "Пользователь отключён")
		return
	}
	connections, err := s.App.Connections(r.Context(), id)
	if err != nil {
		handleErr(w, err)
		return
	}
	for _, c := range connections {
		if c.Kind != kind {
			continue
		}
		value, filename := c.Link, "txt"
		if c.Format == "conf" {
			value, filename = c.Configuration, "conf"
		}
		if r.URL.Query().Get("format") == "qr" {
			svg, e := qrcode.SVG(value)
			if e != nil {
				fail(w, 422, "QR слишком большой")
				return
			}
			w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
			w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'")
			w.Header().Set("Cache-Control", "no-store")
			_, _ = w.Write(svg)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="arcvpn-user-%d-%s.%s"`, id, kind, filename))
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte(value + "\n"))
		return
	}
	fail(w, 404, "У пользователя нет активного подключения этого протокола")
}

func (s *Server) protocolHealth(w http.ResponseWriter, r *http.Request) {
	states, err := s.App.LocalProtocolHealth(r.Context())
	if err != nil {
		handleErr(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	respond(w, 200, map[string]any{"items": states, "note": "server service status only; client reachability not measured"})
}
