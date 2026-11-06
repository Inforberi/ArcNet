package httpapi

import (
	"net/http"
	"strings"

	"github.com/arcvpn/arcvpn/internal/nodeagent"
	"github.com/arcvpn/arcvpn/internal/store"
)

func (s *Server) cascadeProbeGrant(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Version int64  `json:"expected_version"`
		Confirm string `json:"confirm"`
	}
	if !decode(w, r, &input) {
		return
	}
	if input.Confirm != "CREATE_EXTERNAL_PROBE" {
		fail(w, 400, "Подтвердите выгрузку одноразового тестового доступа")
		return
	}
	origin, e := nodeagent.ValidateOrigin(s.origin(r))
	if e != nil {
		fail(w, 409, "Задайте публичный HTTPS-адрес панели перед внешней проверкой")
		return
	}
	bundle, e := s.App.CreateCascadeProbe(r.Context(), r.PathValue("id"), input.Version)
	if e != nil {
		cascadeError(w, e)
		return
	}
	bundle.Controller = origin
	s.App.Store.Event(r.Context(), "cascade.probe.grant", "issued external grant for "+bundle.DeploymentID)
	respond(w, 201, bundle)
}
func (s *Server) cascadeProbeReport(w http.ResponseWriter, r *http.Request) {
	const prefix = "CascadeProbe "
	authorization := r.Header.Get("Authorization")
	if !strings.HasPrefix(authorization, prefix) {
		http.Error(w, "Unauthorized", 401)
		return
	}
	var report store.CascadeProbeReport
	if !decode(w, r, &report) {
		return
	}
	if e := s.App.Store.CompleteCascadeProbe(r.Context(), strings.TrimPrefix(authorization, prefix), report); e != nil {
		fail(w, 409, "Отчёт просрочен, уже использован или ревизии изменились")
		return
	}
	s.App.Store.Event(r.Context(), "cascade.probe.report", "recorded external result for "+report.DeploymentID)
	respond(w, 200, map[string]bool{"recorded": true})
}
