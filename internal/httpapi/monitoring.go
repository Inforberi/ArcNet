package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/arcvpn/arcvpn/internal/auth"
	"github.com/arcvpn/arcvpn/internal/store"
)

// Notifications are coalesced. Slow subscribers never block the collector and
// reconnect by reading current state, rather than replaying obsolete metrics.
type monitoringHub struct {
	mu      sync.Mutex
	clients map[chan struct{}]struct{}
	wake    chan struct{}
	done    chan struct{}
	once    sync.Once
}

func newMonitoringHub() *monitoringHub {
	return &monitoringHub{clients: make(map[chan struct{}]struct{}), wake: make(chan struct{}, 1), done: make(chan struct{})}
}
func (h *monitoringHub) subscribe() (chan struct{}, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.clients) >= 32 {
		return nil, false
	}
	c := make(chan struct{}, 1)
	h.clients[c] = struct{}{}
	return c, true
}
func (h *monitoringHub) unsubscribe(c chan struct{}) {
	h.mu.Lock()
	delete(h.clients, c)
	h.mu.Unlock()
}
func (h *monitoringHub) notify() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients {
		select {
		case c <- struct{}{}:
		default:
		}
	}
}

func (s *Server) monitoringSettings(w http.ResponseWriter, r *http.Request) {
	n, err := s.App.Store.MonitoringSeconds(r.Context())
	if err != nil {
		fail(w, 500, "Не удалось прочитать интервал")
		return
	}
	respond(w, 200, map[string]any{"interval_seconds": n, "available_seconds": []int{5, 10, 15, 30, 60}})
}
func (s *Server) monitoringSave(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Interval int `json:"interval_seconds"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !store.ValidMonitoringSeconds(in.Interval) {
		fail(w, 400, "Выберите 5, 10, 15, 30 или 60 секунд")
		return
	}
	if err := s.App.Store.SetMonitoringSeconds(r.Context(), in.Interval); err != nil {
		fail(w, 500, "Не удалось сохранить интервал")
		return
	}
	select {
	case s.monitoring.wake <- struct{}{}:
	default:
	}
	s.monitoring.notify()
	s.monitoringSettings(w, r)
}

func (s *Server) monitoringStream(w http.ResponseWriter, r *http.Request) {
	// EventSource is same-origin and sends the existing HttpOnly session cookie.
	// No bearer credentials or subscription secrets are placed in the URL.
	if !sameOrigin(r) {
		fail(w, 403, "Origin rejected")
		return
	}
	if _, ok := w.(http.Flusher); !ok {
		fail(w, 503, "Streaming unavailable")
		return
	}
	c, ok := s.monitoring.subscribe()
	if !ok {
		w.Header().Set("Retry-After", "5")
		fail(w, 429, "Слишком много открытых панелей")
		return
	}
	defer s.monitoring.unsubscribe(c)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("X-Accel-Buffering", "no")
	controller := http.NewResponseController(w)
	write := func(event string, payload any) bool {
		// Preserve bounded writes without applying the ordinary 30s timeout to
		// the lifetime of a healthy event stream.
		if err := controller.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil && !errors.Is(err, http.ErrNotSupported) {
			return false
		}
		blob, err := json.Marshal(payload)
		if err != nil {
			return false
		}
		if _, err = fmt.Fprintf(w, "retry: 5000\nevent: %s\ndata: %s\n\n", event, blob); err != nil {
			return false
		}
		if controller.Flush() != nil {
			return false
		}
		_ = controller.SetWriteDeadline(time.Time{})
		return true
	}
	cookie, _ := r.Cookie("arcvpn_session")
	token := auth.HashToken(cookie.Value)
	send := func() bool {
		_, valid, err := s.App.Store.Session(r.Context(), token)
		if err != nil {
			return false
		}
		if !valid {
			write("unauthorized", struct{}{})
			return false
		}
		n, err := s.App.Store.MonitoringSeconds(r.Context())
		if err != nil {
			return false
		}
		return write("refresh", map[string]any{"interval_seconds": n, "sampled_at": time.Now().Unix()})
	}
	if !send() {
		return
	}
	// Keep proxies alive and re-check revoked/expired sessions even when the
	// selected cadence is one minute. Keepalive does not trigger collection.
	keepalive := time.NewTicker(15 * time.Second)
	defer keepalive.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-s.monitoring.done:
			return
		case <-c:
			if !send() {
				return
			}
		case <-keepalive.C:
			_, valid, err := s.App.Store.Session(r.Context(), token)
			if err != nil {
				return
			}
			if !valid {
				write("unauthorized", struct{}{})
				return
			}
			if !write("keepalive", struct{}{}) {
				return
			}
		}
	}
}
