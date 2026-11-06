package httpapi

import (
	"errors"
	"net/http"

	"github.com/arcvpn/arcvpn/internal/store"
	"github.com/arcvpn/arcvpn/internal/usage"
)

func (s *Server) nodeUsage(w http.ResponseWriter, r *http.Request) {
	var report usage.NodeReport
	if !decode(w, r, &report) {
		return
	}
	secret := agentBearer(r)
	if secret == "" || report.NodeID == "" {
		http.Error(w, "Unauthorized", 401)
		return
	}
	e := s.App.RecordNodeUsage(r.Context(), report.NodeID, secret, report)
	if errors.Is(e, store.ErrNodeUnauthorized) {
		http.Error(w, "Unauthorized", 401)
		return
	}
	if e != nil {
		fail(w, 409, "Node traffic report is stale, unsupported or does not match applied configuration")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
