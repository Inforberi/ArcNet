package httpapi

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/arcvpn/arcvpn/internal/backup"
	"github.com/arcvpn/arcvpn/internal/qrcode"
	"github.com/arcvpn/arcvpn/internal/store"
	"github.com/arcvpn/arcvpn/internal/xray"
)

func (s *Server) origin(r *http.Request) string {
	if s.publicURL != "" {
		return s.publicURL
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}
func (s *Server) userURL(r *http.Request, id int64) (string, string, error) {
	user, err := s.App.Store.ByID(r.Context(), id)
	if err != nil {
		return "", "", err
	}
	if !user.Enabled {
		return "", "", store.ErrNotFound
	}
	cfg, err := s.App.Store.ReadReality(r.Context())
	if err != nil {
		return "", "", err
	}
	return s.origin(r) + "/sub/" + user.Subscription, xray.Link(cfg, user), nil
}
func (s *Server) userExport(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	linkURL, _, err := s.userURL(r, id)
	if err != nil {
		handleErr(w, err)
		return
	}
	user, err := s.App.Store.ByID(r.Context(), id)
	if err != nil {
		handleErr(w, err)
		return
	}
	links, err := s.App.SubscriptionLinks(r.Context(), user)
	if err != nil {
		handleErr(w, err)
		return
	}
	contentList := strings.Join(links, "\n") + "\n"
	format := r.URL.Query().Get("format")
	var content string
	switch format {
	case "raw":
		content = contentList
	case "base64":
		content = base64.StdEncoding.EncodeToString([]byte(contentList))
	case "url":
		content = linkURL + "\n"
	default:
		fail(w, 400, "Поддерживаются форматы: raw, base64, url")
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="arcvpn-user-%d-%s.txt"`, id, format))
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.WriteString(w, content)
}
func (s *Server) userQRCode(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	url, link, err := s.userURL(r, id)
	if err != nil {
		handleErr(w, err)
		return
	}
	var value string
	switch r.URL.Query().Get("format") {
	case "subscription":
		value = url
	case "vless":
		value = link
	default:
		fail(w, 400, "Выбери subscription или vless")
		return
	}
	image, err := qrcode.SVG(value)
	if err != nil {
		fail(w, 422, "Не удалось создать QR-код")
		return
	}
	w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'; style-src 'none'")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(image)
}
func (s *Server) backupExport(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Password string `json:"password"`
	}
	if !decode(w, r, &in) {
		return
	}
	snapshot, err := s.App.ExportState(r.Context())
	if err != nil {
		handleErr(w, err)
		return
	}
	encrypted, err := backup.Encrypt(snapshot, in.Password)
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	s.App.Store.Event(r.Context(), "backup.exported", fmt.Sprintf("users=%d", len(snapshot.Users)))
	respond(w, 200, map[string]string{"filename": fmt.Sprintf("arcvpn-state-%s.json", time.Now().UTC().Format("20060102-150405")), "content": string(encrypted)})
}
func readBackupRequest(w http.ResponseWriter, r *http.Request) (string, []byte, string, string, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, backup.MaxArchiveBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	var in struct {
		Password   string `json:"password"`
		Archive    string `json:"archive"`
		Confirm    string `json:"confirm,omitempty"`
		NewAddress string `json:"new_address,omitempty"`
	}
	if err := dec.Decode(&in); err != nil {
		fail(w, 400, "Неверный формат или размер архива")
		return "", nil, "", "", false
	}
	if len(in.Archive) > backup.MaxArchiveBytes || len(in.Password) > 1024 {
		fail(w, 413, "Архив слишком большой")
		return "", nil, "", "", false
	}
	var extra any
	if dec.Decode(&extra) != io.EOF {
		fail(w, 400, "Некорректное содержимое запроса")
		return "", nil, "", "", false
	}
	return in.Password, []byte(in.Archive), in.Confirm, in.NewAddress, true
}
func (s *Server) backupPreview(w http.ResponseWriter, r *http.Request) {
	password, blob, _, newAddress, ok := readBackupRequest(w, r)
	if !ok {
		return
	}
	snapshot, err := backup.Decrypt(blob, password)
	if err != nil {
		fail(w, 422, "Не удалось проверить архив: неверный пароль, повреждённые данные или неподдерживаемая версия")
		return
	}
	if newAddress != "" {
		snapshot, err = snapshot.RemapAddress(newAddress)
		if err != nil {
			fail(w, 422, "Некорректный адрес нового VPS")
			return
		}
	}
	current, err := s.App.ExportState(r.Context())
	if err != nil {
		handleErr(w, err)
		return
	}
	summary := map[string]any{"version": snapshot.Version, "created_at": snapshot.CreatedAt, "users": len(snapshot.Users), "current_users": len(current.Users), "vpn_configured": snapshot.VPN != nil, "replace_required": true, "protocols": len(snapshot.Protocols), "current_protocols": len(current.Protocols), "nodes": len(snapshot.Nodes), "current_nodes": len(current.Nodes), "log_budget_mb": snapshot.LogBudgetMB}
	if snapshot.VPN != nil {
		summary["vpn_address"] = snapshot.VPN.Address
		summary["vpn_port"] = snapshot.VPN.Port
	}
	summary["routes"] = len(snapshot.Routes)
	summary["current_routes"] = len(current.Routes)
	summary["routes_replaced"] = snapshot.Version >= 4
	summary["routing_profiles"] = len(snapshot.RoutingProfiles)
	summary["current_routing_profiles"] = len(current.RoutingProfiles)
	summary["routing_profiles_replaced"] = snapshot.Version >= 5
	respond(w, 200, summary)
}
func (s *Server) backupImport(w http.ResponseWriter, r *http.Request) {
	password, blob, confirm, newAddress, ok := readBackupRequest(w, r)
	if !ok {
		return
	}
	if confirm != "REPLACE_ARCVPN_STATE" {
		fail(w, 400, "Подтверди замену текущего VPN и всех VPN-пользователей")
		return
	}
	snapshot, err := backup.Decrypt(blob, password)
	if err != nil {
		fail(w, 422, "Не удалось расшифровать и проверить архив")
		return
	}
	if snapshot.VPN == nil {
		fail(w, 422, "Восстановление пустой VPN-конфигурации не поддерживается")
		return
	}
	if newAddress != "" {
		snapshot, err = snapshot.RemapAddress(newAddress)
		if err != nil {
			fail(w, 422, "Некорректный адрес нового VPS")
			return
		}
	}
	if err = s.App.RestoreState(r.Context(), snapshot); err != nil {
		if errors.Is(err, store.ErrRouteInvalid) {
			fail(w, 422, "Архив несовместим с текущими маршрутами. Старые архивы не заменяют маршруты: сначала замените или удалите ссылки на отсутствующие ноды")
			return
		}
		var invalid *json.UnmarshalTypeError
		if errors.Is(err, backup.ErrInvalid) || errors.As(err, &invalid) {
			fail(w, 422, "Неподдерживаемая конфигурация архива")
			return
		}
		// Do not leak secrets or Xray output to the browser.
		s.log.Error("backup restore failed", "error", err)
		fail(w, 500, "Не удалось применить настройки. Проверь журнал и состояние Xray")
		return
	}
	if snapshot.LogBudgetMB != nil && s.logRotator != nil {
		if err := s.logRotator.SetBudgetMB(*snapshot.LogBudgetMB); err != nil {
			s.log.Warn("log budget not applied to active writer", "error", err)
		}
	}
	respond(w, 200, map[string]any{"ok": true, "restored_users": len(snapshot.Users), "restored_nodes": len(snapshot.Nodes)})
}

// Intentionally no direct 3x-ui database import: its database schema and
// protocol backends are not compatible with this version of ArcVPN.
