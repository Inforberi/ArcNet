package httpapi

import (
	"github.com/arcvpn/arcvpn/internal/ops"
	"net/http"
)

func (s *Server) journalBudgetGet(w http.ResponseWriter, r *http.Request) {
	limit, err := ops.JournalBudget(r.Context(), s.App.Core.ControlSocket, 0)
	if err != nil {
		fail(w, 503, "Настройки изолированного журнала недоступны")
		return
	}
	respond(w, 200, map[string]any{"budget_mb": limit, "scope": "arcvpn_journal_namespace", "system_journal_unchanged": true})
}
func (s *Server) journalBudgetSave(w http.ResponseWriter, r *http.Request) {
	var in struct {
		BudgetMB int `json:"budget_mb"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.BudgetMB < 50 || in.BudgetMB > 1024 {
		fail(w, 400, "Лимит от 50 до 1024 МБ")
		return
	}
	limit, err := ops.JournalBudget(r.Context(), s.App.Core.ControlSocket, in.BudgetMB)
	if err != nil {
		fail(w, 503, "Не удалось применить лимит изолированного журнала")
		return
	}
	s.App.Store.Event(r.Context(), "journald.limit", "ArcVPN journal namespace budget changed")
	respond(w, 200, map[string]int{"budget_mb": limit})
}
