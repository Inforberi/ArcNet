package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/arcvpn/arcvpn/internal/routes"
	"github.com/arcvpn/arcvpn/internal/store"
)

func nodeRouteConflict(w http.ResponseWriter, err error) bool {
	var dependency *store.NodeRouteDependencyError
	if !errors.As(err, &dependency) {
		return false
	}
	fail(w, http.StatusConflict, "Сначала замените ноду или удалите черновики маршрутов: "+strings.Join(dependency.Names, ", "))
	return true
}

// Routing stage 5 starts as a draft-only control plane. No endpoint here can
// modify Xray, manipulate the published subscription, or claim probe success.
func (s *Server) routeList(w http.ResponseWriter, r *http.Request) {
	policies, e := s.App.Store.RoutePolicies(r.Context())
	if e != nil {
		fail(w, 500, "Не удалось прочитать маршруты")
		return
	}
	respond(w, 200, map[string]any{"items": policies, "edges": routes.Edges(policies), "stage": "draft_only", "publication_enabled": false})
}
func routeError(w http.ResponseWriter, e error) {
	switch {
	case errors.Is(e, store.ErrNotFound):
		fail(w, 404, "Маршрут не найден")
	case errors.Is(e, store.ErrCascadeBusy):
		fail(w, 409, "Маршрут связан с журналом каскада; сначала откатите операцию и удалите её журнал")
	case errors.Is(e, store.ErrConflict):
		fail(w, 409, "Конфликт версий маршрута; обновите данные")
	case errors.Is(e, store.ErrRouteInvalid):
		fail(w, 400, "Некорректная политика: "+e.Error())
	default:
		fail(w, 500, "Операция с маршрутом не выполнена")
	}
}
func (s *Server) routePreview(w http.ResponseWriter, r *http.Request) {
	var p routes.Policy
	if !decode(w, r, &p) {
		return
	}
	edges, changes, e := s.App.Store.PreviewRoute(r.Context(), p)
	if e != nil {
		routeError(w, e)
		return
	}
	respond(w, 200, map[string]any{"valid": true, "edges": edges, "changes": changes, "side_effects": false, "publication_enabled": false})
}
func (s *Server) routeCreate(w http.ResponseWriter, r *http.Request) {
	var p routes.Policy
	if !decode(w, r, &p) {
		return
	}
	// Ignore client-supplied IDs and revisions; only the server allocates them.
	p.ID = ""
	p.Version = 0
	p.UpdatedAt = 0
	created, e := s.App.SaveRoute(r.Context(), p, 0)
	if e != nil {
		routeError(w, e)
		return
	}
	s.App.Store.Event(r.Context(), "route.draft.created", "routing draft "+created.ID)
	respond(w, 201, created)
}
func (s *Server) routeGet(w http.ResponseWriter, r *http.Request) {
	p, e := s.App.Store.RoutePolicy(r.Context(), r.PathValue("id"))
	if e != nil {
		routeError(w, e)
		return
	}
	respond(w, 200, p)
}
func (s *Server) routeUpdate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Policy          routes.Policy `json:"policy"`
		ExpectedVersion int64         `json:"expected_version"`
	}
	if !decode(w, r, &req) {
		return
	}
	if req.ExpectedVersion < 1 {
		fail(w, 400, "Укажите ожидаемую версию")
		return
	}
	req.Policy.ID = r.PathValue("id")
	updated, e := s.App.SaveRoute(r.Context(), req.Policy, req.ExpectedVersion)
	if e != nil {
		routeError(w, e)
		return
	}
	s.App.Store.Event(r.Context(), "route.draft.updated", "routing draft "+updated.ID)
	respond(w, 200, updated)
}
func (s *Server) routeDelete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ExpectedVersion int64 `json:"expected_version"`
	}
	if !decode(w, r, &req) {
		return
	}
	e := s.App.DeleteRoute(r.Context(), r.PathValue("id"), req.ExpectedVersion)
	if e != nil {
		routeError(w, e)
		return
	}
	s.App.Store.Event(r.Context(), "route.draft.deleted", "routing draft "+r.PathValue("id"))
	respond(w, 200, map[string]bool{"deleted": true})
}
func (s *Server) routeSimulate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Samples []routes.Sample `json:"samples"`
	}
	if !decode(w, r, &req) {
		return
	}
	p, e := s.App.Store.RoutePolicy(r.Context(), r.PathValue("id"))
	if e != nil {
		routeError(w, e)
		return
	}
	decisions, e := routes.Simulate(p, req.Samples)
	if e != nil {
		fail(w, 400, "Недопустимые тестовые наблюдения: "+e.Error())
		return
	}
	respond(w, 200, map[string]any{"simulated": true, "verified": false, "published": false, "decisions": decisions})
}
