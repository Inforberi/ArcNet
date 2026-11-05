package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/arcvpn/arcvpn/internal/nodebootstrap"
	"github.com/arcvpn/arcvpn/internal/store"
)

func (s *Server) nodeList(w http.ResponseWriter, r *http.Request) {
	nodes, err := s.App.Store.ListNodes(r.Context())
	if err != nil {
		fail(w, 500, "Не удалось получить список нод")
		return
	}
	respond(w, 200, map[string]any{"items": nodes, "node_mode": "managed_v1", "controller": s.publicURL})
}
func (s *Server) nodeEnroll(w http.ResponseWriter, r *http.Request) {
	if s.publicURL == "" {
		fail(w, 409, "Настрой публичный URL панели для подключения нод")
		return
	}
	var data struct {
		Name    string `json:"name"`
		Address string `json:"address"`
		Port    int    `json:"port"`
	}
	if !decode(w, r, &data) {
		return
	}
	var secret string
	var err error
	if data.Address == "" {
		secret, err = s.App.Store.CreateEnrollment(r.Context(), data.Name)
	} else {
		publicIP, lookupErr := nodebootstrap.ResolvePublic(r.Context(), data.Address)
		if lookupErr != nil {
			fail(w, 400, "Адрес ноды должен быть публичным IPv4/IPv6 или разрешаемым публичным DNS-именем")
			return
		}
		secret, err = s.App.Store.CreateEnrollmentForNode(r.Context(), data.Name, publicIP, data.Port)
	}
	if err != nil {
		fail(w, 400, "Некорректное название ноды")
		return
	}
	s.App.Store.Event(r.Context(), "node.enrollment", "one-time enrollment issued")
	respond(w, 201, map[string]any{"token": secret, "expires_seconds": 600, "controller": s.publicURL})
}
func (s *Server) nodeRegister(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	var data struct {
		Token string `json:"token"`
	}
	if !decode(w, r, &data) {
		return
	}
	node, secret, err := s.App.Store.RegisterNode(ctx, data.Token)
	if err != nil {
		if errors.Is(err, store.ErrEnrollmentExpired) {
			fail(w, 403, "Недействительный или истёкший код регистрации")
			return
		}
		fail(w, 500, "Registration failed")
		return
	}
	s.App.Store.Event(ctx, "node.register", "registered node: "+node.Name)
	respond(w, 201, map[string]string{"node_id": node.ID, "secret": secret})
}
func (s *Server) nodeHeartbeat(w http.ResponseWriter, r *http.Request) {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	var hb store.NodeHeartbeat
	if !decode(w, r, &hb) {
		return
	}
	if err := s.App.Store.Heartbeat(r.Context(), strings.TrimPrefix(auth, "Bearer "), hb); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		http.Error(w, "Invalid heartbeat", http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) nodeDelete(w http.ResponseWriter, r *http.Request) {
	if err := s.App.Store.RemoveNode(r.Context(), r.PathValue("id")); err != nil {
		if nodeRouteConflict(w, err) {
			return
		}
		if errors.Is(err, store.ErrNotFound) {
			fail(w, 404, "Нода не найдена")
			return
		}
		fail(w, 500, "Ошибка удаления ноды")
		return
	}
	s.App.Store.Event(r.Context(), "node.remove", "node identity revoked")
	respond(w, 200, map[string]bool{"ok": true})
}

// Distribution deliberately serves only the running signed-transport executable.
// It is not an arbitrary file server or remote command endpoint.
func (s *Server) nodeBinary(w http.ResponseWriter, r *http.Request) {
	const prefix = "Enrollment "
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, prefix) {
		http.Error(w, "Unauthorized", 401)
		return
	}
	valid, err := s.App.Store.EnrollmentValid(r.Context(), strings.TrimPrefix(h, prefix))
	if err != nil {
		http.Error(w, "Internal error", 500)
		return
	}
	if !valid {
		http.Error(w, "Unauthorized", 401)
		return
	}
	bin, e := os.Executable()
	if e != nil {
		http.Error(w, "not available", 503)
		return
	}
	f, e := os.Open(bin)
	if e != nil {
		http.Error(w, "not available", 503)
		return
	}
	defer f.Close()
	stat, e := f.Stat()
	if e != nil || !stat.Mode().IsRegular() {
		http.Error(w, "not available", 503)
		return
	}
	if r.URL.Query().Get("checksum") == "1" {
		hash := sha256.New()
		if _, e := io.Copy(hash, f); e != nil {
			http.Error(w, "not available", 503)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = io.WriteString(w, hex.EncodeToString(hash.Sum(nil))+"\n")
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename=arcvpn-node-linux-amd64")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, "arcvpn-node-linux-amd64", stat.ModTime(), f)
}
