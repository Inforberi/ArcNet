package httpapi

import (
	"bytes"
	"errors"
	"net/http"
)

// Buffer only finite credential/config responses, never event streams.
type exportResponse struct {
	header   http.Header
	body     bytes.Buffer
	status   int
	overflow bool
}

func (w *exportResponse) Header() http.Header { return w.header }
func (w *exportResponse) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}
func (w *exportResponse) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	if w.body.Len()+len(p) > 4<<20 {
		w.overflow = true
		return 0, errors.New("export exceeds limit")
	}
	return w.body.Write(p)
}
func (s *Server) stableExports(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		before := s.App.PublicationVersion()
		busy := func() {
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Retry-After", "1")
			fail(w, 503, "Подключения обновляются. Повторите запрос через несколько секунд.")
		}
		if before%2 != 0 {
			busy()
			return
		}
		buffer := &exportResponse{header: make(http.Header)}
		next(buffer, r)
		if before != s.App.PublicationVersion() {
			busy()
			return
		}
		if buffer.overflow {
			fail(w, 500, "Не удалось подготовить подключение")
			return
		}
		for key, values := range buffer.header {
			w.Header()[key] = values
		}
		status := buffer.status
		if status == 0 {
			status = http.StatusOK
		}
		w.WriteHeader(status)
		_, _ = w.Write(buffer.body.Bytes())
	}
}
