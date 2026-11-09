package httpapi

import (
	"context"
	"crypto/ed25519"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"github.com/arcvpn/arcvpn/internal/nodeagent"
	"github.com/arcvpn/arcvpn/internal/ops"
	"github.com/arcvpn/arcvpn/internal/releaseversion"
	"github.com/arcvpn/arcvpn/internal/updater"
)

type downloadProgress struct {
	Version string `json:"version"`
	State   string `json:"state"`
	Message string `json:"message,omitempty"`
	cancel  context.CancelFunc
}
type updateManager struct {
	progress     atomic.Pointer[downloadProgress]
	slots        chan struct{}
	catalogSlots chan struct{}
	pending      atomic.Bool
	pendingStart atomic.Uint64
	publicKey    string
	stagePath    string
	version      string
	platform     string
	catalog      func(context.Context, ed25519.PublicKey) (updater.Catalog, error)
	download     func(context.Context, updater.Release, ed25519.PublicKey) ([]byte, error)
}

func newUpdateManager() updateManager {
	return updateManager{slots: make(chan struct{}, 1), catalogSlots: make(chan struct{}, 2), publicKey: updater.PublicKeyFile, stagePath: updateStagePath(), version: nodeagent.Version, platform: runtime.GOOS + "/" + runtime.GOARCH, catalog: updater.FetchCatalog, download: updater.Download}
}
func updateStagePath() string {
	directory := os.Getenv("ARCVPN_DATA_DIR")
	if directory == "" {
		directory = "/var/lib/arcvpn"
	}
	return filepath.Join(directory, "update", "staged.zip")
}
func (s *Server) updateStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	state, err := ops.Update(r.Context(), s.App.Core.ControlSocket, "status")
	if err != nil {
		respond(w, 200, map[string]any{"available": false, "trusted": false, "staged": false, "current_version": s.updates.version, "message": "Служба обновлений недоступна. Для старой версии нужен первоначальный переход на новый обработчик."})
		return
	}
	if !state.Busy && state.StartedAt > s.updates.pendingStart.Load() && state.Result != "" && state.Result != "success" {
		s.updates.pending.Store(false)
	}
	respond(w, 200, map[string]any{"available": true, "trusted": state.Trusted, "staged": state.Staged, "version": state.Version, "current_version": s.updates.version, "applying": state.Busy || s.updates.pending.Load(), "result": state.Result, "started_at": state.StartedAt, "download": s.updates.progress.Load()})
}
func (s *Server) beginUpdate(w http.ResponseWriter, r *http.Request) bool {
	select {
	case s.updates.slots <- struct{}{}:
	default:
		fail(w, 409, "Другая операция обновления уже выполняется")
		return false
	}
	state, err := ops.Update(r.Context(), s.App.Core.ControlSocket, "status")
	if err != nil || !state.Trusted || state.Busy || s.updates.pending.Load() {
		<-s.updates.slots
		fail(w, 409, "Обновление занято или доверенная служба недоступна. Проверьте состояние.")
		return false
	}
	return true
}
func (s *Server) stageUpdate(ctx context.Context, data []byte, expected string, signed []byte) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	pub, err := updater.ReadPublicKey(s.updates.publicKey)
	if err != nil {
		return "", err
	}
	payload, err := updater.VerifyPublished(data, signed, pub, s.updates.version, s.updates.platform)
	if err != nil {
		return "", err
	}
	if !updater.IsUpgrade(s.updates.version, payload.Manifest.Version) {
		return "", updater.ErrInvalid
	}
	if expected != "" && payload.Manifest.Version != expected {
		return "", updater.ErrInvalid
	}
	path := s.updates.stagePath
	if _, err = os.Lstat(path); err == nil {
		return "", errors.New("already staged")
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return "", err
	}
	catFile, err := os.CreateTemp(filepath.Dir(path), ".catalog-*")
	if err != nil {
		return "", err
	}
	catName := catFile.Name()
	defer os.Remove(catName)
	if _, err = catFile.Write(signed); err != nil {
		catFile.Close()
		return "", err
	}
	if err = catFile.Sync(); err != nil {
		catFile.Close()
		return "", err
	}
	if err = catFile.Close(); err != nil {
		return "", err
	}
	if err = os.Rename(catName, filepath.Join(filepath.Dir(path), "catalog.json")); err != nil {
		return "", err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".upload-*")
	if err != nil {
		return "", err
	}
	name := file.Name()
	defer os.Remove(name)
	if _, err = file.Write(data); err != nil {
		file.Close()
		return "", err
	}
	if err = file.Sync(); err != nil {
		file.Close()
		return "", err
	}
	if err = file.Close(); err != nil {
		return "", err
	}
	if err = os.Rename(name, path); err != nil {
		return "", err
	}
	state, err := ops.Update(ctx, s.App.Core.ControlSocket, "verify")
	if err != nil || state.Version != payload.Manifest.Version {
		_ = os.Remove(path)
		return "", updater.ErrInvalid
	}
	s.App.Store.Event(ctx, "update.staged", "Verified signed ArcNet release "+state.Version)
	return state.Version, nil
}
func (s *Server) updateUpload(w http.ResponseWriter, r *http.Request) {
	if !s.beginUpdate(w, r) {
		return
	}
	defer func() { <-s.updates.slots }()
	if r.Header.Get("Content-Type") != "application/octet-stream" {
		fail(w, 415, "Ожидается подписанный ZIP")
		return
	}
	if r.ContentLength == 0 || r.ContentLength > updater.MaxArchive {
		fail(w, 413, "Недопустимый размер архива")
		return
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, updater.MaxArchive))
	if err != nil {
		fail(w, 413, "Архив слишком большой или повреждён")
		return
	}
	pub, keyErr := updater.ReadPublicKey(s.updates.publicKey)
	if keyErr != nil {
		fail(w, 409, "Недоступен доверенный ключ")
		return
	}
	catalog, catalogErr := s.verifiedCatalog(r.Context(), pub)
	if catalogErr != nil {
		catalogFailure(w, catalogErr)
		return
	}
	version, err := s.stageUpdate(r.Context(), data, "", catalog.Signed)
	if err != nil {
		fail(w, 422, "Архив не прошёл проверку или уже подготовлена другая версия. Действующий подготовленный пакет сохранён.")
		return
	}
	respond(w, 200, map[string]any{"version": version, "verified": true})
}

var errCatalogBusy = errors.New("catalog busy")

func (s *Server) verifiedCatalog(ctx context.Context, pub ed25519.PublicKey) (updater.Catalog, error) {
	select {
	case s.updates.catalogSlots <- struct{}{}:
		defer func() { <-s.updates.catalogSlots }()
		return s.updates.catalog(ctx, pub)
	default:
		return updater.Catalog{}, errCatalogBusy
	}
}
func catalogFailure(w http.ResponseWriter, err error) {
	if errors.Is(err, errCatalogBusy) {
		w.Header().Set("Retry-After", "2")
		fail(w, 429, "Проверка версий уже выполняется. Повторите через несколько секунд.")
		return
	}
	fail(w, 502, "Не удалось получить проверенный список версий. Повторите проверку позже.")
}
func (s *Server) updateCatalog(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	pub, err := updater.ReadPublicKey(s.updates.publicKey)
	if err != nil {
		fail(w, 409, "Не настроен доверенный ключ обновлений")
		return
	}
	catalog, err := s.verifiedCatalog(r.Context(), pub)
	if err != nil {
		catalogFailure(w, err)
		return
	}
	items := make([]updater.Release, 0, len(catalog.Releases))
	type historical struct {
		updater.Release
		Installable bool   `json:"installable"`
		Reason      string `json:"reason"`
		Prerelease  bool   `json:"prerelease"`
	}
	history := make([]historical, 0, len(catalog.Releases))
	platform := s.updates.platform
	for _, item := range catalog.Releases {
		reason := ""
		allowed := item.Compatible(s.updates.version, platform)
		if !allowed {
			reason = "Совместимость перехода не подтверждена"
			if item.Version == s.updates.version {
				reason = "Установлена сейчас"
			} else if !updater.IsUpgrade(s.updates.version, item.Version) {
				reason = "Возврат заблокирован: нет проверенного восстановления БД и конфигурации"
			} else if item.Platform != platform {
				reason = "Другая платформа"
			}
		}
		history = append(history, historical{item, allowed, reason, releaseversion.Prerelease(item.Version)})
		if item.Compatible(s.updates.version, platform) {
			items = append(items, item)
		}
	}
	respond(w, 200, map[string]any{"items": items, "history": history, "history_partial": catalog.Partial, "current_version": s.updates.version, "platform": platform})
}
func (s *Server) updateHistory(w http.ResponseWriter, r *http.Request) {
	select {
	case s.updates.catalogSlots <- struct{}{}:
		defer func() { <-s.updates.catalogSlots }()
	default:
		catalogFailure(w, errCatalogBusy)
		return
	}
	rows, err := updater.FetchReleaseHistory(r.Context())
	if err != nil {
		fail(w, 502, "История GitHub недоступна. Подписанные версии показаны отдельно.")
		return
	}
	respond(w, 200, map[string]any{"items": rows, "informational_only": true})
}
func (s *Server) updateDownload(w http.ResponseWriter, r *http.Request) {
	if !s.beginUpdate(w, r) {
		return
	}
	handedOff := false
	defer func() {
		if !handedOff {
			<-s.updates.slots
		}
	}()
	var in struct {
		Version string `json:"version"`
	}
	if !decode(w, r, &in) {
		return
	}
	if _, err := os.Lstat(s.updates.stagePath); err == nil {
		fail(w, 409, "Уже подготовлен пакет. Установите его или отмените подготовку.")
		return
	} else if !errors.Is(err, os.ErrNotExist) {
		fail(w, 503, "Каталог обновления недоступен")
		return
	}
	pub, err := updater.ReadPublicKey(s.updates.publicKey)
	if err != nil {
		fail(w, 409, "Не настроен доверенный ключ")
		return
	}
	catalog, err := s.updates.catalog(r.Context(), pub)
	if err != nil {
		catalogFailure(w, err)
		return
	}
	for _, release := range catalog.Releases {
		if release.Version != in.Version || !release.Compatible(s.updates.version, s.updates.platform) {
			continue
		}
		jobCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		s.updates.progress.Store(&downloadProgress{Version: release.Version, State: "downloading", cancel: cancel})
		go func() {
			defer cancel()
			defer func() { <-s.updates.slots }()
			data, err := s.updates.download(jobCtx, release, pub)
			if err == nil {
				if jobCtx.Err() != nil {
					err = jobCtx.Err()
				} else {
					_, err = s.stageUpdate(jobCtx, data, release.Version, catalog.Signed)
				}
			}
			state, message := "verified", ""
			if err != nil {
				state = "failed"
				message = "Загрузка отменена или пакет не прошёл проверку. Можно повторить."
			}
			s.updates.progress.Store(&downloadProgress{Version: release.Version, State: state, Message: message})
		}()
		handedOff = true
		respond(w, 202, map[string]any{"accepted": true, "version": release.Version})
		return
	}
	fail(w, 409, "Эта версия не является совместимым обновлением для установленной панели")
}
func (s *Server) updateCancelDownload(w http.ResponseWriter, r *http.Request) {
	p := s.updates.progress.Load()
	if p == nil || p.State != "downloading" {
		fail(w, 409, "Активной загрузки нет")
		return
	}
	p.cancel()
	respond(w, 202, map[string]bool{"accepted": true})
}

func (s *Server) updateApply(w http.ResponseWriter, r *http.Request) {
	if !s.beginUpdate(w, r) {
		return
	}
	defer func() { <-s.updates.slots }()
	var in struct {
		Confirm string `json:"confirm"`
		Version string `json:"version"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !strings.EqualFold(in.Confirm, "INSTALL_SIGNED_UPDATE") {
		fail(w, 400, "Требуется подтверждение обновления")
		return
	}
	verified, err := ops.Update(r.Context(), s.App.Core.ControlSocket, "verify")
	if err != nil || (in.Version != "" && verified.Version != in.Version) {
		fail(w, 409, "Подготовленная версия изменилась. Проверьте пакет заново.")
		return
	}
	state, err := ops.Update(r.Context(), s.App.Core.ControlSocket, "apply")
	if err != nil || !state.Accepted {
		fail(w, 503, "Не удалось запустить проверенное обновление")
		return
	}
	s.updates.pendingStart.Store(verified.StartedAt)
	s.updates.pending.Store(true)
	s.App.Store.Event(r.Context(), "update.started", "Signed ArcNet release "+state.Version)
	respond(w, 202, map[string]any{"accepted": true, "version": state.Version, "started_at": verified.StartedAt})
}
func (s *Server) updateDiscard(w http.ResponseWriter, r *http.Request) {
	if !s.beginUpdate(w, r) {
		return
	}
	defer func() { <-s.updates.slots }()
	if err := os.Remove(s.updates.stagePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		fail(w, 503, "Не удалось удалить подготовленный архив")
		return
	}
	respond(w, 200, map[string]bool{"ok": true})
}
