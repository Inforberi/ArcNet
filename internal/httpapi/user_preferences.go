package httpapi

import (
	"errors"
	"net/http"

	"github.com/arcvpn/arcvpn/internal/store"
)

func (s *Server) userPreferences(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	if r.Method == "GET" {
		u, err := s.App.Store.ByID(r.Context(), id)
		if err != nil {
			handleErr(w, err)
			return
		}
		respond(w, 200, map[string]any{"name": u.Name, "preferences": u.Preferences})
		return
	}
	var in struct {
		Name            string                `json:"name"`
		Preferences     store.UserPreferences `json:"preferences"`
		ExpectedVersion *int64                `json:"expected_version"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.ExpectedVersion == nil {
		fail(w, 400, "Нужна текущая версия пользователя")
		return
	}
	if err := s.App.EditUserPreferences(r.Context(), id, in.Name, in.Preferences, *in.ExpectedVersion); err != nil {
		switch {
		case errors.Is(err, store.ErrInvalidUserPreferences):
			fail(w, 422, "Проверьте имя, название подписки и выбранные подключения")
		case errors.Is(err, store.ErrConflict):
			fail(w, 409, "Пользователь уже изменён или имя занято. Обновите данные перед сохранением")
		case errors.Is(err, store.ErrUserAssignmentsUnavailable):
			fail(w, 422, "Выбор подключений пока доступен только без удалённых нод, ожидающих регистраций и AmneziaWG")
		default:
			activationFailure(w, err)
		}
		return
	}
	u, err := s.App.Store.ByID(r.Context(), id)
	if err != nil {
		handleErr(w, err)
		return
	}
	respond(w, 200, map[string]any{"name": u.Name, "preferences": u.Preferences})
}
