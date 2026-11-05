package httpapi

import (
	"errors"
	"github.com/arcvpn/arcvpn/internal/ops"
	"github.com/arcvpn/arcvpn/internal/updater"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func updateStagePath() string {
	directory := os.Getenv("ARCVPN_DATA_DIR")
	if directory == "" {
		directory = "/var/lib/arcvpn"
	}
	return filepath.Join(directory, "update", "staged.zip")
}
func (s *Server) updateStatus(w http.ResponseWriter, r *http.Request) {
	state, err := ops.Update(r.Context(), s.App.Core.ControlSocket, "status")
	if err != nil {
		respond(w, 200, map[string]any{"available": false, "trusted": false, "staged": false, "message": "Управление релизами доступно только после установки на VPS"})
		return
	}
	respond(w, 200, map[string]any{"available": true, "trusted": state.Trusted, "staged": state.Staged, "version": state.Version})
}
func (s *Server) updateUpload(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Content-Type") != "application/octet-stream" {
		fail(w, 415, "Ожидается бинарный архив update.zip")
		return
	}
	if r.ContentLength == 0 || r.ContentLength > updater.MaxArchive {
		fail(w, 413, "Недопустимый размер архива")
		return
	}
	path := updateStagePath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		fail(w, 503, "Каталог обновлений недоступен")
		return
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".upload-*")
	if err != nil {
		fail(w, 503, "Не удалось создать временный файл")
		return
	}
	name := file.Name()
	defer os.Remove(name)
	_, err = io.Copy(file, http.MaxBytesReader(w, r.Body, updater.MaxArchive))
	if err != nil {
		file.Close()
		fail(w, 413, "Архив превышает допустимый размер или повреждён")
		return
	}
	if err = file.Sync(); err != nil {
		file.Close()
		fail(w, 503, "Не удалось записать архив")
		return
	}
	if err = file.Close(); err != nil {
		fail(w, 503, "Не удалось завершить запись")
		return
	}
	if err = os.Rename(name, path); err != nil {
		fail(w, 503, "Не удалось сохранить архив")
		return
	}
	state, err := ops.Update(r.Context(), s.App.Core.ControlSocket, "verify")
	if err != nil {
		// A bad or unsigned archive must not remain staged for later application.
		_ = os.Remove(path)
		fail(w, 422, "Архив не подписан доверенным ключом или не прошёл проверку")
		return
	}
	s.App.Store.Event(r.Context(), "update.staged", "Verified signed ArcVPN release")
	respond(w, 200, map[string]any{"version": state.Version, "verified": true})
}
func (s *Server) updateApply(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Confirm string `json:"confirm"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !strings.EqualFold(in.Confirm, "INSTALL_SIGNED_UPDATE") {
		fail(w, 400, "Требуется подтверждение обновления")
		return
	}
	state, err := ops.Update(r.Context(), s.App.Core.ControlSocket, "apply")
	if err != nil || !state.Accepted {
		fail(w, 503, "Не удалось запустить проверенное обновление")
		return
	}
	s.App.Store.Event(r.Context(), "update.started", "Signed ArcVPN release apply requested")
	respond(w, 202, map[string]any{"accepted": true, "version": state.Version, "notice": "Сервис будет перезапущен. В случае отказа предыдущий бинарник восстановится автоматически."})
}
func (s *Server) updateDiscard(w http.ResponseWriter, r *http.Request) {
	path := updateStagePath()
	err := os.Remove(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		fail(w, 503, "Не удалось удалить подготовленный архив")
		return
	}
	respond(w, 200, map[string]bool{"ok": true})
}
