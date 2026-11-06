package httpapi

import (
	"errors"
	"net/http"

	"github.com/arcvpn/arcvpn/internal/store"
)

func cascadeError(w http.ResponseWriter, e error) {
	switch {
	case errors.Is(e, store.ErrCascadeNotReady):
		fail(w, 409, "Нужны актуальные подтверждённые конфиги и обновлённые агент/helper обеих нод. Проверьте их доступность и режим обслуживания.")
	case errors.Is(e, store.ErrCascadeBusy):
		fail(w, 409, "Нода или маршрут заняты каскадом. Завершите операцию или откатите её; затем удалите завершённый журнал.")
	case errors.Is(e, store.ErrUsageUnsupported):
		fail(w, 409, "Каскад пока доступен только без режима устройств и конечных пакетов: распределённый контроль ещё не подключён.")
	case errors.Is(e, store.ErrConflict):
		fail(w, 409, "План или версия операции изменились. Обновите состояние и повторите предпросмотр.")
	case errors.Is(e, store.ErrRouteInvalid):
		fail(w, 400, "Выберите два удалённых сервера, каскад без резервов с блокировкой при отказе и свободный межнодовый порт 1024–65535.")
	case errors.Is(e, store.ErrNotFound):
		fail(w, 404, "Маршрут, подключение или операция не найдены")
	default:
		fail(w, 409, "Не удалось подготовить каскад. Проверьте конфиги и ключи нод; обновите журнал перед повтором.")
	}
}
func (s *Server) cascadeList(w http.ResponseWriter, r *http.Request) {
	items, e := s.App.Store.CascadeDeployments(r.Context())
	if e != nil {
		cascadeError(w, e)
		return
	}
	respond(w, 200, map[string]any{"items": items, "publication_enabled": false})
}
func (s *Server) cascadePreview(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Version int64 `json:"expected_version"`
		Port    int   `json:"channel_port"`
	}
	if !decode(w, r, &input) {
		return
	}
	plan, e := s.App.CascadePreview(r.Context(), r.PathValue("id"), input.Version, input.Port)
	if e != nil {
		cascadeError(w, e)
		return
	}
	respond(w, 200, plan)
}
func (s *Server) cascadeApply(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Version int64  `json:"expected_version"`
		Port    int    `json:"channel_port"`
		Token   string `json:"preview_token"`
		Confirm string `json:"confirm"`
	}
	if !decode(w, r, &input) {
		return
	}
	if input.Confirm != "APPLY_CASCADE" {
		fail(w, 400, "Подтвердите применение всего TCP/REALITY-подключения")
		return
	}
	deployment, e := s.App.ApplyCascadeFromPreview(r.Context(), r.PathValue("id"), input.Version, input.Port, input.Token)
	if e != nil {
		cascadeError(w, e)
		return
	}
	s.App.Store.Event(r.Context(), "cascade.apply", "queued cascade "+deployment.ID)
	respond(w, 202, deployment)
}
func (s *Server) cascadeAction(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Version int64  `json:"expected_version"`
		Confirm string `json:"confirm"`
	}
	if !decode(w, r, &input) {
		return
	}
	id := r.PathValue("id")
	action := r.PathValue("action")
	switch action {
	case "start":
		if input.Confirm != "APPLY_CASCADE" {
			fail(w, 400, "Подтвердите применение")
			return
		}
		deployment, e := s.App.StartPreparedCascade(r.Context(), id, input.Version)
		if e != nil {
			cascadeError(w, e)
			return
		}
		s.App.Store.Event(r.Context(), "cascade.start", "resumed cascade "+id)
		respond(w, 202, deployment)
	case "rollback":
		if input.Confirm != "ROLLBACK_CASCADE" {
			fail(w, 400, "Подтвердите откат")
			return
		}
		if e := s.App.RollbackCascade(r.Context(), id, input.Version); e != nil {
			cascadeError(w, e)
			return
		}
		s.App.Store.Event(r.Context(), "cascade.rollback", "rollback cascade "+id)
		respond(w, 202, map[string]bool{"queued": true})
	default:
		fail(w, 404, "Неизвестная операция каскада")
	}
}
func (s *Server) cascadeDelete(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Version int64  `json:"expected_version"`
		Confirm string `json:"confirm"`
	}
	if !decode(w, r, &input) {
		return
	}
	if input.Confirm != "DELETE_CASCADE_JOURNAL" {
		fail(w, 400, "Подтвердите удаление журнала")
		return
	}
	id := r.PathValue("id")
	if e := s.App.DeleteCascade(r.Context(), id, input.Version); e != nil {
		cascadeError(w, e)
		return
	}
	s.App.Store.Event(r.Context(), "cascade.journal.deleted", "deleted cascade "+id)
	respond(w, 200, map[string]bool{"deleted": true})
}
