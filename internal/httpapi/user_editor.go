package httpapi

import (
	"errors"
	"github.com/arcvpn/arcvpn/internal/store"
	"github.com/arcvpn/arcvpn/internal/usage"
	"net/http"
)

func userEditError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrConflict):
		fail(w, 409, "Данные уже изменены или имя занято. Обновите список и откройте редактор заново")
	case errors.Is(err, store.ErrNotFound):
		fail(w, 404, "Пользователь не найден; изменения не сохранены")
	case errors.Is(err, store.ErrInvalidUserPreferences), errors.Is(err, usage.ErrInvalid):
		fail(w, 422, "Проверьте имя, подключения и параметры пакета")
	case errors.Is(err, store.ErrUsageUnsupported), errors.Is(err, store.ErrUserAssignmentsUnavailable):
		fail(w, 422, "Эти ограничения пока недоступны с удалёнными нодами или неподдерживаемыми протоколами")
	default:
		fail(w, 422, "Изменения не сохранены. Проверьте доступность служб и свежесть учёта трафика")
	}
}
func (s *Server) userEditor(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	if r.Method == "GET" {
		u, p, err := s.App.Store.UserEditor(r.Context(), id)
		if err != nil {
			handleErr(w, err)
			return
		}
		respond(w, 200, map[string]any{"name": u.Name, "enabled": u.Enabled, "preferences": u.Preferences, "package": p})
		return
	}
	var input struct {
		store.UserEdit
		ExpectedVersion *int64 `json:"expected_version"`
		ExpectedEnabled *bool  `json:"expected_enabled"`
		Enabled         *bool  `json:"enabled"`
	}
	if !decode(w, r, &input) {
		return
	}
	if input.ExpectedVersion == nil || input.ExpectedEnabled == nil || input.Enabled == nil {
		fail(w, 400, "Нужны текущая версия и состояние доступа")
		return
	}
	in := input.UserEdit
	in.ExpectedVersion = *input.ExpectedVersion
	in.ExpectedEnabled = *input.ExpectedEnabled
	in.Enabled = *input.Enabled
	if err := s.App.SaveUserEditor(r.Context(), id, in); err != nil {
		userEditError(w, err)
		return
	}
	state, _ := s.App.Store.AccountingState(r.Context())
	respond(w, 200, map[string]any{"saved": true, "runtime_state": state})
}
func (s *Server) usersBulkAccess(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Items   []store.UserSelection `json:"items"`
		Enabled *bool                 `json:"enabled"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Enabled == nil {
		fail(w, 400, "Укажите действие")
		return
	}
	if err := s.App.BulkUserAccess(r.Context(), in.Items, *in.Enabled); err != nil {
		userEditError(w, err)
		return
	}
	state, _ := s.App.Store.AccountingState(r.Context())
	respond(w, 200, map[string]any{"saved": true, "count": len(in.Items), "runtime_state": state})
}
