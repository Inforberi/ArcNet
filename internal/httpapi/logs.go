package httpapi

import (
	"net/http"
)

func (s *Server) logSettings(w http.ResponseWriter, r *http.Request) {
	limit, err := s.App.Store.LogBudgetMB(r.Context())
	if err != nil {
		handleErr(w, err)
		return
	}
	var used int64
	if s.logRotator != nil {
		used = s.logRotator.UsedBytes()
	}
	respond(w, 200, map[string]any{"budget_mb": limit, "used_bytes": used, "scope": "panel_only", "xray_journal_managed": false})
}
func (s *Server) logSettingsSave(w http.ResponseWriter, r *http.Request) {
	var in struct {
		BudgetMB int `json:"budget_mb"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.BudgetMB < 50 || in.BudgetMB > 1024 {
		fail(w, 400, "Лимит должен быть от 50 до 1024 МБ")
		return
	}
	old, err := s.App.Store.LogBudgetMB(r.Context())
	if err != nil {
		handleErr(w, err)
		return
	}
	if s.logRotator != nil {
		if err = s.logRotator.SetBudgetMB(in.BudgetMB); err != nil {
			handleErr(w, err)
			return
		}
	}
	if err = s.App.Store.SetLogBudgetMB(r.Context(), in.BudgetMB); err != nil {
		if s.logRotator != nil {
			_ = s.logRotator.SetBudgetMB(old)
		}
		handleErr(w, err)
		return
	}
	s.App.Store.Event(r.Context(), "logs.limit", "application log limit changed")
	respond(w, 200, map[string]any{"ok": true, "budget_mb": in.BudgetMB})
}
func (s *Server) logTail(w http.ResponseWriter, r *http.Request) {
	if s.logRotator == nil {
		respond(w, 200, map[string]any{"text": "", "available": false})
		return
	}
	output, err := s.logRotator.Tail(16 << 10)
	if err != nil {
		handleErr(w, err)
		return
	}
	respond(w, 200, map[string]any{"text": output, "available": true})
}
func (s *Server) logClear(w http.ResponseWriter, r *http.Request) {
	if s.logRotator == nil {
		fail(w, 503, "Локальные журналы не настроены")
		return
	}
	if err := s.logRotator.Clear(); err != nil {
		handleErr(w, err)
		return
	}
	s.App.Store.Event(r.Context(), "logs.cleared", "application log files cleared")
	respond(w, 200, map[string]bool{"ok": true})
}
