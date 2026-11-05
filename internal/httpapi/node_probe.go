package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/arcvpn/arcvpn/internal/store"
)

func (s *Server) nodeProbeGrant(w http.ResponseWriter, r *http.Request) {
	grant, err := s.App.Store.CreateProbeGrant(r.Context(), r.PathValue("id"))
	switch {
	case errors.Is(err, store.ErrNotFound):
		fail(w, http.StatusNotFound, "Нода не найдена")
	case errors.Is(err, store.ErrConflict):
		fail(w, http.StatusConflict, "Нода не готова к внешней проверке: нужна актуальная применённая ревизия и отключённый drain")
	case err != nil:
		fail(w, http.StatusInternalServerError, "Не удалось создать одноразовую проверку")
	default:
		s.App.Store.Event(r.Context(), "node.probe.grant", "external probe grant issued for node "+grant.NodeID)
		respond(w, http.StatusCreated, grant)
	}
}

func (s *Server) nodeProbeReport(w http.ResponseWriter, r *http.Request) {
	const prefix = "Probe "
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, prefix) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	var report store.ProbeReport
	if !decode(w, r, &report) {
		return
	}
	control, err := s.App.Store.CompleteProbe(r.Context(), strings.TrimPrefix(header, prefix), report)
	switch {
	case errors.Is(err, store.ErrProbeGrant):
		http.Error(w, "Invalid or expired probe grant", http.StatusUnauthorized)
	case errors.Is(err, store.ErrConflict):
		http.Error(w, "Node revision changed; probe is stale", http.StatusConflict)
	case err != nil:
		http.Error(w, "Unable to store probe result", http.StatusInternalServerError)
	default:
		s.App.Store.Event(r.Context(), "node.probe.report", "external probe result="+control.Probe+" for node "+control.NodeID)
		respond(w, http.StatusOK, map[string]any{"node_id": control.NodeID, "vpn_probe": control.Probe, "probe_revision": control.ProbeRevision, "probe_checked_at": control.ProbeCheckedAt})
	}
}
