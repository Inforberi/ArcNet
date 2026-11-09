package httpapi

import (
	"github.com/arcvpn/arcvpn/internal/usage"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *Server) userTraffic(w http.ResponseWriter, r *http.Request) {
	rawIDs := r.URL.Query().Get("ids")
	if len(rawIDs) > 4199 {
		fail(w, 400, "Слишком длинный список пользователей")
		return
	}
	parts := strings.Split(rawIDs, ",")
	if len(parts) < 1 || len(parts) > 200 {
		fail(w, 400, "За один запрос можно получить до 200 пользователей")
		return
	}
	ids := make(map[int64]bool, len(parts))
	for _, part := range parts {
		id, err := strconv.ParseInt(part, 10, 64)
		if err != nil || id < 1 || ids[id] {
			fail(w, 400, "Проверьте список пользователей")
			return
		}
		ids[id] = true
	}
	requested := make([]int64, 0, len(ids))
	for id := range ids {
		requested = append(requested, id)
	}
	users, err := s.App.Store.TrafficUsers(r.Context(), requested)
	if err != nil {
		handleErr(w, err)
		return
	}
	owners := make(map[int64]string, len(users))
	for _, u := range users {
		owners[u.ID] = u.Identity
	}
	now := time.Now().Unix()
	items, complete := s.App.UserTraffic.Snapshot(now, users)
	presence, summary := s.App.UserPresence.Snapshot(users, now)
	attach := func(item *usage.UserTraffic) {
		p := presence[item.UserID]
		item.Presence = &p
		item.Online = nil
		if p.State == "online" || p.State == "offline" {
			online := p.State == "online"
			item.Online = &online
		}
	}
	for i := range items {
		attach(&items[i])
	}
	result := map[string]any{"items": items, "sampled_at": now, "fresh_for": usage.FreshFor, "poll_seconds": 5, "source": "local-engine-counters", "complete_coverage": complete, "history_seconds": usage.TrafficHistorySeconds, "history_persistent": false}
	result["presence_summary"] = summary
	if raw := r.URL.Query().Get("history_id"); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || owners[id] == "" {
			fail(w, 400, "История доступна только для пользователя из текущего запроса")
			return
		}
		current, history, coverage := s.App.UserTraffic.Detail(id, owners[id], now)
		attach(&current)
		result["detail"] = map[string]any{"current": current, "history": history, "complete_coverage": coverage}
	}
	w.Header().Set("Cache-Control", "no-store")
	respond(w, 200, result)
}
