package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/arcvpn/arcvpn/internal/app"
	"github.com/arcvpn/arcvpn/internal/protocols"
	"github.com/arcvpn/arcvpn/internal/store"
	"github.com/arcvpn/arcvpn/internal/xray"
)

type protocolView struct {
	store.Protocol
	Revision string `json:"revision"`
}

func connectionEditFailure(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, app.ErrConnectionChanged):
		fail(w, 409, "Настройки уже изменились. Закройте форму и откройте её заново.")
	case errors.Is(err, protocols.ErrInvalid):
		fail(w, 422, "Проверьте порт, SNI, путь, HTTP Host и дополнительные настройки выбранного протокола. Значения должны укладываться в указанные диапазоны, а порт — быть свободным для этого протокола.")
	case errors.Is(err, store.ErrLocalServerInUse):
		fail(w, 409, "Это последний локальный протокол для существующих пользователей. Оставьте его отключённым или сначала добавьте другой.")
	case errors.Is(err, store.ErrConflict), strings.Contains(err.Error(), "disable protocol"):
		fail(w, 409, "Сначала отключите протокол. Удаление также недоступно при зависимых legacy-подключениях.")
	case errors.Is(err, store.ErrNotFound):
		handleErr(w, err)
	default:
		activationFailure(w, err)
	}
}

func (s *Server) connectionEdit(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Path          *string              `json:"path,omitempty"`
		Fingerprint   *string              `json:"fingerprint,omitempty"`
		ServerOptions *store.ServerOptions `json:"server_options,omitempty"`
		ClientOptions *store.ClientOptions `json:"client_options,omitempty"`
		Revision      string               `json:"revision"`
		Port          int                  `json:"port"`
		Target        string               `json:"target"`
		SNI           string               `json:"sni"`
		Enabled       *bool                `json:"enabled"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Fingerprint != nil && !xray.ValidFingerprint(*in.Fingerprint) {
		fail(w, 422, "Выберите поддерживаемый TLS fingerprint клиента")
		return
	}
	if in.Enabled == nil || len(in.Revision) != 64 {
		fail(w, 400, "Нужны текущая версия настроек и состояние протокола")
		return
	}
	if err := s.App.EditConnection(r.Context(), r.PathValue("kind"), in.Revision, app.ConnectionSettings{ServerOptions: in.ServerOptions, ClientOptions: in.ClientOptions, Path: in.Path, Fingerprint: in.Fingerprint, Port: in.Port, Target: in.Target, SNI: in.SNI, Enabled: *in.Enabled}); err != nil {
		connectionEditFailure(w, err)
		return
	}
	respond(w, 200, map[string]bool{"ok": true})
}
func (s *Server) connectionDelete(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Revision string `json:"revision"`
	}
	if !decode(w, r, &in) {
		return
	}
	if len(in.Revision) != 64 {
		fail(w, 400, "Нужна текущая версия настроек")
		return
	}
	kind := r.PathValue("kind")
	var err error
	if kind == "vless-reality" {
		err = s.App.DeleteReality(r.Context(), in.Revision)
	} else if kind == protocols.XHTTP || kind == protocols.Hysteria2 || kind == protocols.WireGuard || store.IsAdditionalConnection(kind) {
		err = s.App.DeleteProtocolRevision(r.Context(), kind, in.Revision)
	} else {
		err = protocols.ErrInvalid
	}
	if err != nil {
		connectionEditFailure(w, err)
		return
	}
	respond(w, 200, map[string]bool{"ok": true})
}
