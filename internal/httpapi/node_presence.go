package httpapi

import (
	"errors"
	"github.com/arcvpn/arcvpn/internal/store"
	"github.com/arcvpn/arcvpn/internal/usage"
	"net/http"
)

func (s *Server) nodePresence(w http.ResponseWriter, r *http.Request) {
	var report usage.NodePresenceReport
	if !decode(w, r, &report) {
		return
	}
	secret := agentBearer(r)
	if secret == "" || report.NodeID == "" {
		http.Error(w, "Unauthorized", 401)
		return
	}
	err := s.App.RecordNodePresence(r.Context(), report.NodeID, secret, report)
	if errors.Is(err, store.ErrNodeUnauthorized) {
		http.Error(w, "Unauthorized", 401)
		return
	}
	if err != nil {
		fail(w, 409, "Node presence is unsupported, stale or does not match applied configuration")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
