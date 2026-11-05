package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/arcvpn/arcvpn/internal/devices"
	"github.com/arcvpn/arcvpn/internal/store"
)

func (s *Server) userDevices(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	record, err := s.App.Store.UserDevices(r.Context(), id)
	if err != nil {
		handleErr(w, err)
		return
	}
	type item struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Created int64  `json:"created_at"`
		Revoked int64  `json:"revoked_at"`
	}
	items := make([]item, 0, len(record.Devices))
	for _, d := range record.Devices {
		items = append(items, item{d.ID, d.Name, d.CreatedAt, d.RevokedAt})
	}
	runtime, err := s.App.Store.AccountingState(r.Context())
	if err != nil {
		handleErr(w, err)
		return
	}
	coverage := s.App.Store.CheckPackageCoverage(r.Context(), 1, 0)
	if coverage != nil && !errors.Is(coverage, store.ErrUsageUnsupported) {
		handleErr(w, coverage)
		return
	}
	respond(w, 200, map[string]any{"items": items, "managed": record.Version > 0, "limit": record.Limit, "version": record.Version, "active": record.Active(), "supported": coverage == nil, "runtime_state": runtime, "physical_devices_verified": false})
}
func (s *Server) userDeviceChange(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var in struct {
		Name    string `json:"name"`
		Version int64  `json:"expected_version"`
		Limit   *int   `json:"limit"`
	}
	if !decode(w, r, &in) {
		return
	}
	var limit *int
	revoke := ""
	name := ""
	switch r.Method {
	case http.MethodPost:
		name = in.Name
	case http.MethodPut:
		if in.Limit == nil {
			fail(w, 400, "Укажите лимит")
			return
		}
		limit = in.Limit
	case http.MethodDelete:
		revoke = r.PathValue("device")
	}
	if err := s.App.ChangeUserDevices(r.Context(), id, in.Version, name, revoke, limit); err != nil {
		switch {
		case errors.Is(err, devices.ErrInvalid):
			fail(w, 400, "Некорректное имя или лимит (0–100)")
		case errors.Is(err, devices.ErrLimit):
			fail(w, 409, "Лимит доступов достигнут. Сначала отзовите лишние доступы или увеличьте лимит.")
		case errors.Is(err, store.ErrUsageUnsupported):
			fail(w, 409, "Отдельные доступы пока доступны только для локальных Xray и Hysteria2 без удалённых нод и других активных движков.")
		case errors.Is(err, store.ErrConflict):
			fail(w, 409, "Список изменён. Обновите данные перед повтором.")
		default:
			handleErr(w, err)
		}
		return
	}
	s.userDevices(w, r)
}
func (s *Server) userDeviceLinks(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	links, err := s.App.DeviceLinks(r.Context(), id, r.PathValue("device"))
	if err != nil {
		handleErr(w, err)
		return
	}
	record, err := s.App.Store.UserDevices(r.Context(), id)
	if err != nil {
		handleErr(w, err)
		return
	}
	for _, d := range record.Devices {
		if d.ID == r.PathValue("device") && d.RevokedAt == 0 {
			respond(w, 200, map[string]any{"url": s.origin(r) + "/device/" + d.Token, "links": links})
			return
		}
	}
	http.NotFound(w, r)
}
func (s *Server) deviceSubscription(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	if len(token) != 43 {
		http.NotFound(w, r)
		return
	}
	id, deviceID, err := s.App.Store.DeviceByToken(r.Context(), token)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	links, err := s.App.DeviceLinks(r.Context(), id, deviceID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	s.subscriptionUsage(w, r, id)
	_, _ = w.Write([]byte(strings.Join(links, "\n") + "\n"))
}
