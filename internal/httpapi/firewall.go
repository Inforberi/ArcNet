package httpapi

import (
	"errors"
	"github.com/arcvpn/arcvpn/internal/ops"
	"github.com/arcvpn/arcvpn/internal/protocols"
	"github.com/arcvpn/arcvpn/internal/store"
	"net/http"
)

type portEndpoint struct {
	Kind      string `json:"kind"`
	Port      int    `json:"port"`
	Transport string `json:"transport"`
	Enabled   bool   `json:"enabled"`
}

func (s *Server) firewallEndpoints(r *http.Request) ([]portEndpoint, error) {
	result := make([]portEndpoint, 0, 5)
	vpn, err := s.App.Store.ReadReality(r.Context())
	if err == nil {
		result = append(result, portEndpoint{Kind: "vless-reality", Port: vpn.Port, Transport: "tcp", Enabled: vpn.Enabled})
	} else if !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	extras, err := s.App.Store.ListProtocols(r.Context())
	if err != nil {
		return nil, err
	}
	for _, p := range extras {
		result = append(result, portEndpoint{Kind: p.Kind, Port: p.Port, Transport: protocols.Transport(p.Kind), Enabled: p.Enabled})
	}
	return result, nil
}
func (s *Server) firewallStatus(w http.ResponseWriter, r *http.Request) {
	ends, err := s.firewallEndpoints(r)
	if err != nil {
		handleErr(w, err)
		return
	}
	status, err := ops.Firewall(r.Context(), s.App.Core.ControlSocket, "status", ops.FirewallRule{})
	if err != nil {
		respond(w, 200, map[string]any{"available": false, "writable": false, "endpoints": ends, "message": "Не удалось прочитать UFW. Проверьте наличие UFW и совместимость системного обработчика; состояние портов неизвестно."})
		return
	}
	respond(w, 200, map[string]any{"available": true, "writable": false, "active": status.Active, "rules": status.Rules, "endpoints": ends, "message": "Показаны правила ArcNet. Другие правила и firewall провайдера здесь не проверяются. Изменение правил через панель ожидает проверки безопасного отката."})
}
func (s *Server) firewallSave(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Port      int    `json:"port"`
		Transport string `json:"transport"`
		Allow     bool   `json:"allow"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Port == 22 || in.Port == 80 || in.Port == s.App.ReservedTCPPort || in.Port < 1 || in.Port > 65535 || (in.Transport != "tcp" && in.Transport != "udp") {
		fail(w, 400, "Изменять этот порт через VPN firewall нельзя")
		return
	}
	endpoints, err := s.firewallEndpoints(r)
	if err != nil {
		handleErr(w, err)
		return
	}
	if in.Allow {
		found := false
		for _, end := range endpoints {
			if end.Port == in.Port && end.Transport == in.Transport && end.Enabled {
				found = true
			}
		}
		if !found {
			fail(w, 409, "Открывать порт можно только для включённого VPN-протокола")
			return
		}
	}
	action := "remove"
	if in.Allow {
		action = "allow"
	}
	status, err := ops.Firewall(r.Context(), s.App.Core.ControlSocket, action, ops.FirewallRule{Port: in.Port, Transport: in.Transport})
	if err != nil {
		fail(w, 503, "UFW недоступен или изменение правил отклонено. Проверь журнал и текущее состояние сервера")
		return
	}
	s.App.Store.Event(r.Context(), "firewall."+action, "VPN port rule changed")
	respond(w, 200, status)
}
