package httpapi

import (
	"context"
	"crypto/ed25519"
	"errors"
	"net/http"
	"time"

	"github.com/arcvpn/arcvpn/internal/publicnet"
	"github.com/arcvpn/arcvpn/internal/store"
	"github.com/arcvpn/arcvpn/internal/targetcheck"
	"github.com/arcvpn/arcvpn/internal/updater"
)

// The verification key is independently provisioned by the operator, never
// accepted from a downloaded catalog or a browser request.
func (s *Server) WithTargetCatalogKey(path string) error {
	key, err := updater.ReadPublicKey(path)
	if err != nil {
		return err
	}
	s.targetCatalogKey = append(ed25519.PublicKey{}, key...)
	return nil
}
func (s *Server) targetCatalog(w http.ResponseWriter, r *http.Request) {
	c, err := s.App.Store.TargetCatalog(r.Context())
	if err != nil {
		fail(w, 500, "Не удалось прочитать каталог целей")
		return
	}
	respond(w, 200, map[string]any{"catalog": c, "signed_update_enabled": len(s.targetCatalogKey) == ed25519.PublicKeySize, "candidates_verified": false})
}
func (s *Server) targetCatalogImport(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Text            string `json:"text"`
		URL             string `json:"url"`
		ExpectedVersion int64  `json:"expected_version"`
	}
	if !decodeRouting(w, r, &req) {
		return
	}
	old, err := s.App.Store.TargetCatalog(r.Context())
	if err != nil {
		fail(w, 500, "Не удалось прочитать каталог")
		return
	}
	if old.Version != req.ExpectedVersion {
		fail(w, 409, "Каталог изменён; обновите список")
		return
	}
	var catalog targetcheck.Catalog
	if req.URL != "" {
		if req.Text != "" || len(s.targetCatalogKey) != ed25519.PublicKeySize {
			fail(w, 400, "Для HTTPS-обновления нужен независимо установленный ключ каталога; укажите только один источник")
			return
		}
		select {
		case s.targetSlots <- struct{}{}:
			defer func() { <-s.targetSlots }()
		default:
			fail(w, 429, "Другие сетевые проверки уже выполняются")
			return
		}
		blob, e := publicnet.New().Fetch(r.Context(), req.URL, targetcheck.MaxCatalogBytes*2)
		if e != nil {
			fail(w, 400, e.Error())
			return
		}
		catalog, err = targetcheck.VerifyCatalog(blob, s.targetCatalogKey)
		if err != nil {
			fail(w, 400, err.Error())
			return
		}
		catalog.Source = req.URL
	} else {
		candidates, e := targetcheck.ParseCandidates(req.Text)
		if e != nil {
			fail(w, 400, e.Error())
			return
		}
		catalog = targetcheck.Catalog{Version: old.Version + 1, Candidates: candidates, Source: "manual"}
	}
	if err = s.App.Store.SaveTargetCatalog(r.Context(), catalog, req.ExpectedVersion); err != nil {
		if errors.Is(err, store.ErrConflict) {
			fail(w, 409, "Каталог изменён или подписанная версия устарела")
		} else {
			fail(w, 400, "Не удалось сохранить каталог")
		}
		return
	}
	s.App.Store.Event(r.Context(), "targets.catalog.saved", "target catalog updated; no runtime changes")
	s.targetCatalog(w, r)
}
func (s *Server) targetNodes(w http.ResponseWriter, r *http.Request) {
	nodes, err := s.App.Store.TargetNodes(r.Context())
	if err != nil {
		fail(w, 500, "Не удалось получить ноды")
		return
	}
	respond(w, 200, map[string]any{"items": nodes})
}
func (s *Server) targetCheckStart(w http.ResponseWriter, r *http.Request) {
	var req struct {
		NodeID     string                  `json:"node_id"`
		Candidates []targetcheck.Candidate `json:"candidates"`
	}
	if !decode(w, r, &req) {
		return
	}
	if req.NodeID == "" {
		req.NodeID = "local"
	}
	candidates, err := targetcheck.Normalize(req.Candidates)
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	select {
	case s.targetSlots <- struct{}{}:
		defer func() { <-s.targetSlots }()
	default:
		fail(w, 429, "Другие сетевые проверки уже выполняются")
		return
	}
	job, err := s.App.Store.QueueTargetCheck(r.Context(), req.NodeID, candidates)
	if err != nil {
		if errors.Is(err, store.ErrIncompatibleNodeProtocol) {
			fail(w, 409, "На ноде нет доступного агента с поддержкой проверки SNI")
		} else {
			fail(w, 409, "Проверка уже выполняется или превышен лимит запросов")
		}
		return
	}
	if req.NodeID != "local" {
		respond(w, 202, job)
		return
	}
	report, err := s.targetChecker.Check(r.Context(), job.Candidates, "local")
	if err != nil {
		fail(w, 400, "Не удалось выполнить проверку")
		return
	}
	persist, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err = s.App.Store.CompleteTargetCheck(persist, job.ID, "local", "", job.Digest, report); err != nil {
		fail(w, 500, "Не удалось сохранить результат проверки")
		return
	}
	job, err = s.App.Store.TargetCheck(r.Context(), job.ID)
	if err != nil {
		fail(w, 500, "Не удалось прочитать результат")
		return
	}
	respond(w, 200, job)
}
func (s *Server) targetCheckGet(w http.ResponseWriter, r *http.Request) {
	job, err := s.App.Store.TargetCheck(r.Context(), r.PathValue("id"))
	if err != nil {
		fail(w, 404, "Проверка не найдена")
		return
	}
	respond(w, 200, job)
}
func (s *Server) nodeTargetPoll(w http.ResponseWriter, r *http.Request) {
	nodeID := r.URL.Query().Get("node_id")
	token := agentBearer(r)
	if nodeID == "local" || nodeID == "" || token == "" {
		http.Error(w, "Unauthorized", 401)
		return
	}
	if err := s.App.Store.TargetHello(r.Context(), nodeID, token); err != nil {
		http.Error(w, "Unauthorized", 401)
		return
	}
	job, err := s.App.Store.PollTargetCheck(r.Context(), nodeID, token)
	if err != nil {
		http.Error(w, "Unable to poll target check", 409)
		return
	}
	if job == nil {
		w.WriteHeader(204)
		return
	}
	respond(w, 200, job)
}
func (s *Server) nodeTargetReport(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID     string             `json:"id"`
		NodeID string             `json:"node_id"`
		Digest string             `json:"digest"`
		Report targetcheck.Report `json:"report"`
	}
	if !decode(w, r, &req) {
		return
	}
	token := agentBearer(r)
	if req.NodeID == "local" || req.NodeID == "" || token == "" {
		http.Error(w, "Unauthorized", 401)
		return
	}
	err := s.App.Store.CompleteTargetCheck(r.Context(), req.ID, req.NodeID, token, req.Digest, req.Report)
	if errors.Is(err, store.ErrNodeUnauthorized) {
		http.Error(w, "Unauthorized", 401)
		return
	}
	if err != nil {
		http.Error(w, "Stale or malformed target report", 409)
		return
	}
	w.WriteHeader(204)
}
