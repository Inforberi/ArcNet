package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/arcvpn/arcvpn/internal/routingprofile"
	"github.com/arcvpn/arcvpn/internal/store"
)

func profileError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrConflict):
		fail(w, 409, "Профиль изменён в другом окне. Обновите серверную версию; ваши правки остаются в форме.")
	case errors.Is(err, store.ErrNotFound):
		fail(w, 404, "Профиль не найден")
	case errors.Is(err, routingprofile.ErrInvalid):
		fail(w, 400, err.Error())
	default:
		fail(w, 500, "Не удалось сохранить профиль")
	}
}
func (s *Server) routingProfiles(w http.ResponseWriter, r *http.Request) {
	profiles, err := s.App.Store.RoutingProfiles(r.Context())
	if err != nil {
		profileError(w, err)
		return
	}
	respond(w, 200, map[string]any{"items": profiles, "server_application_enabled": false, "publication_enabled": false})
}
func (s *Server) routingProfileSave(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Profile         routingprofile.Profile `json:"profile"`
		ExpectedVersion int64                  `json:"expected_version"`
	}
	if !decodeRouting(w, r, &req) {
		return
	}
	status := http.StatusOK
	if r.Method == "POST" {
		req.Profile.ID = ""
		req.ExpectedVersion = 0
		status = http.StatusCreated
	} else {
		if req.ExpectedVersion < 1 {
			fail(w, 400, "Укажите ожидаемую версию")
			return
		}
		req.Profile.ID = r.PathValue("id")
	}
	saved, err := s.App.SaveRoutingProfile(r.Context(), req.Profile, req.ExpectedVersion)
	if err != nil {
		profileError(w, err)
		return
	}
	s.App.Store.Event(r.Context(), "routing.profile.saved", "profile "+saved.ID)
	respond(w, status, saved)
}
func (s *Server) routingProfileDelete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ExpectedVersion int64 `json:"expected_version"`
	}
	if !decodeRouting(w, r, &req) {
		return
	}
	if err := s.App.DeleteRoutingProfile(r.Context(), r.PathValue("id"), req.ExpectedVersion); err != nil {
		profileError(w, err)
		return
	}
	s.App.Store.Event(r.Context(), "routing.profile.deleted", "profile "+r.PathValue("id"))
	respond(w, 200, map[string]bool{"deleted": true})
}
func (s *Server) routingProfileCheck(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Profile routingprofile.Profile `json:"profile"`
		Address string                 `json:"address"`
	}
	if !decodeRouting(w, r, &req) {
		return
	}
	decision, err := routingprofile.Explain(req.Profile, req.Address)
	if err != nil {
		profileError(w, err)
		return
	}
	respond(w, 200, decision)
}
func (s *Server) routingListPreview(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Text string `json:"text"`
	}
	if !decodeRouting(w, r, &req) {
		return
	}
	entries, duplicates, err := routingprofile.ParseList(req.Text)
	if err != nil {
		profileError(w, err)
		return
	}
	respond(w, 200, map[string]any{"entries": entries, "duplicates": duplicates, "count": len(entries), "side_effects": false})
}
func (s *Server) routingProfileImport(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Archive string `json:"archive"`
	}
	if !decodeRouting(w, r, &req) {
		return
	}
	p, err := routingprofile.Decode([]byte(req.Archive))
	if err != nil {
		profileError(w, err)
		return
	}
	respond(w, 200, map[string]any{"profile": p, "side_effects": false})
}
func (s *Server) routingProfileExport(w http.ResponseWriter, r *http.Request) {
	profiles, err := s.App.Store.RoutingProfiles(r.Context())
	if err != nil {
		profileError(w, err)
		return
	}
	for _, p := range profiles {
		if p.ID == r.PathValue("id") {
			w.Header().Set("Content-Disposition", `attachment; filename="arcvpn-routing-profile.json"`)
			respond(w, 200, routingprofile.Bundle{Format: "arcvpn-routing-profile", Version: 1, Profile: p})
			return
		}
	}
	profileError(w, store.ErrNotFound)
}
func decodeRouting(w http.ResponseWriter, r *http.Request, out any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, routingprofile.MaxImportBytes*2)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		fail(w, 400, "Некорректный JSON или превышен размер импорта")
		return false
	}
	var tail any
	if err := dec.Decode(&tail); err != io.EOF {
		fail(w, 400, "Лишние данные после JSON")
		return false
	}
	return true
}
