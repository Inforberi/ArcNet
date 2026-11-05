package httpapi

import (
	"github.com/arcvpn/arcvpn/internal/ops"
	"net/http"
)

func (s *Server) journalTail(w http.ResponseWriter, r *http.Request) {
	service := r.URL.Query().Get("service")
	text, err := ops.JournalTail(r.Context(), s.App.Core.ControlSocket, service)
	if err != nil {
		fail(w, 503, "Журнал службы недоступен. Проверь права и локальный обработчик")
		return
	}
	respond(w, 200, map[string]any{"service": service, "text": text, "scope": "arcvpn_services_only", "retention_managed": false})
}
