package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/arcvpn/arcvpn/internal/store"
	"github.com/arcvpn/arcvpn/internal/usage"
)

func (s *Server) userUsage(w http.ResponseWriter, r *http.Request) {
	packages, err := s.App.Store.Packages(r.Context())
	if err != nil {
		handleErr(w, err)
		return
	}
	fresh, last, err := s.App.Store.AccountingFresh(r.Context())
	if err != nil {
		handleErr(w, err)
		return
	}
	state, err := s.App.Store.AccountingState(r.Context())
	if err != nil {
		handleErr(w, err)
		return
	}
	coverageErr := s.App.Store.CheckPackageCoverage(r.Context(), 1, 0)
	if coverageErr != nil && !errors.Is(coverageErr, store.ErrUsageUnsupported) {
		handleErr(w, coverageErr)
		return
	}
	type item struct {
		usage.Package
		Reason string `json:"block_reason"`
	}
	items := make([]item, 0, len(packages))
	for _, p := range packages {
		items = append(items, item{p, p.Reason(time.Now().Unix(), fresh)})
	}
	respond(w, 200, map[string]any{"items": items, "sampled_at": time.Now().Unix(), "source": "local-engines", "source_fresh": fresh, "source_last_success": last, "complete_coverage": coverageErr == nil, "runtime_state": state, "physical_devices_verified": false})
}
func (s *Server) userPackageSave(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var in struct {
		Quota   int64 `json:"quota_bytes"`
		Expires int64 `json:"expires_at"`
		Version int64 `json:"expected_version"`
		Reset   bool  `json:"reset_usage"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Quota < 0 || in.Quota > usage.MaxBytes || in.Expires < 0 || in.Expires > 253402300799 || in.Version < 0 {
		fail(w, 400, "Некорректный размер пакета, срок или версия")
		return
	}
	if err := s.App.SaveUserPackage(r.Context(), id, in.Quota, in.Expires, in.Version, in.Reset); err != nil {
		switch {
		case errors.Is(err, store.ErrUsageUnsupported):
			fail(w, 409, "Для части подключений ещё нет учёта трафика или исполнения ограничений. Конечный пакет сейчас доступен только для локальных Xray и Hysteria2.")
		case errors.Is(err, store.ErrConflict):
			fail(w, 409, "Пакет изменён. Обновите данные перед сохранением.")
		case errors.Is(err, store.ErrNotFound):
			fail(w, 404, "Пользователь не найден")
		case errors.Is(err, usage.ErrInvalid):
			fail(w, 400, "Некорректные параметры пакета")
		default:
			fail(w, 422, "Не удалось получить свежие счётчики. Пакет не изменён; проверьте доступность VPN-движков.")
		}
		return
	}
	state, err := s.App.Store.AccountingState(r.Context())
	if err != nil {
		handleErr(w, err)
		return
	}
	respond(w, 200, map[string]any{"saved": true, "runtime_state": state, "user_id": id})
}
func (s *Server) userDebits(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if _, err := s.App.Store.ByID(r.Context(), id); err != nil {
		handleErr(w, err)
		return
	}
	items, err := s.App.Store.Debits(r.Context(), id)
	if err != nil {
		handleErr(w, err)
		return
	}
	respond(w, 200, map[string]any{"items": items, "scope": "observed-local-engines", "history_limit": 60})
}
func (s *Server) subscriptionUsage(w http.ResponseWriter, r *http.Request, id int64) {
	if s.App.Store.CheckPackageCoverage(r.Context(), 1, 0) != nil {
		return
	}
	p, err := s.App.Store.Package(r.Context(), id)
	if err != nil {
		return
	}
	fresh, _, err := s.App.Store.AccountingFresh(r.Context())
	if err != nil || !fresh {
		return
	}
	if p.ObservedAt == 0 || p.Incomplete {
		return
	}
	w.Header().Set("Subscription-Userinfo", "upload="+strconv.FormatInt(p.Upload, 10)+"; download="+strconv.FormatInt(p.Download, 10)+"; total="+strconv.FormatInt(p.QuotaBytes, 10)+"; expire="+strconv.FormatInt(p.ExpiresAt, 10))
}
