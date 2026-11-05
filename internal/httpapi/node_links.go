package httpapi

import (
	"errors"
	"net/http"

	"github.com/arcvpn/arcvpn/internal/store"
	"github.com/arcvpn/arcvpn/internal/xray"
)

// Direct VLESS connection links are an explicit administrative diagnostic
// artifact. No link is added to subscriptions and VPN probe remains unknown.
func (s *Server) nodeDirectLinks(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	control, err := s.App.Store.NodeControl(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		fail(w, http.StatusNotFound, "Нода не найдена")
		return
	}
	if err != nil {
		fail(w, 500, "Ошибка чтения ноды")
		return
	}
	if control.Draining {
		fail(w, http.StatusConflict, "Нода выведена из выдачи новых ссылок (drain); уже выданные VPN-ссылки могут продолжать работать")
		return
	}
	if control.Applied == 0 || control.DeploymentStatus != "applied" {
		fail(w, http.StatusConflict, "Сначала дождись подтверждённого применения Xray")
		return
	}
	profile, err := s.App.Store.NodeReality(r.Context(), id)
	if err != nil {
		fail(w, 409, "Невозможно получить Reality-профиль")
		return
	}
	users, err := s.App.Store.ListUsers(r.Context())
	if err != nil {
		fail(w, 500, "Не удалось получить пользователей")
		return
	}
	type item struct {
		UserID int64  `json:"user_id"`
		Name   string `json:"name"`
		Link   string `json:"link"`
	}
	links := make([]item, 0, len(users))
	for _, user := range users {
		if s.App.UserAllowed(r.Context(), user.ID) != nil {
			continue
		}
		links = append(links, item{UserID: user.ID, Name: user.Name, Link: xray.Link(profile, user)})
	}
	respond(w, 200, map[string]any{"node_id": id, "links": links, "vpn_probe": control.Probe, "warning": "Diagnostic links are credentials, not subscription publication; a prior successful external probe applies only to the recorded revision and testing network"})
}
