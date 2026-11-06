package httpapi

import (
	"errors"
	"net/http"
	"net/netip"
	"net/url"

	"github.com/arcvpn/arcvpn/internal/protocols"
	"github.com/arcvpn/arcvpn/internal/store"
)

// Defaults are a read-only proposal. No keys are generated and no listeners or
// firewall rules are changed until the user explicitly creates a connection.
func (s *Server) connectionDefaults(w http.ResponseWriter, r *http.Request) {
	address, source := "", "unknown"
	reality, err := s.App.Store.ReadReality(r.Context())
	if err == nil {
		address, source = reality.Address, "configured"
	} else if !errors.Is(err, store.ErrNotFound) {
		fail(w, 500, "Не удалось прочитать подключения")
		return
	}
	if address == "" && s.publicURL != "" {
		if u, err := url.Parse(s.publicURL); err == nil {
			if ip, err := netip.ParseAddr(u.Hostname()); err == nil && ip.IsGlobalUnicast() && !ip.IsPrivate() {
				address, source = ip.String(), "panel-public-url"
			}
		}
	}
	if server, err := s.App.Store.LocalServer(r.Context()); err == nil {
		address, source = server.Address, "configured"
	} else if !errors.Is(err, store.ErrNotFound) {
		fail(w, 500, "Не удалось прочитать адрес сервера")
		return
	}
	profiles, err := s.App.Store.ListProtocols(r.Context())
	if err != nil {
		fail(w, 500, "Не удалось прочитать подключения")
		return
	}
	ports := map[string]int{}
	for _, kind := range append([]string{"vless-reality"}, protocols.Kinds()...) {
		port := 443
		if kind != "vless-reality" {
			port = protocols.DefaultPort(kind)
		}
		for ; port <= 65535; port++ {
			busy := protocols.Transport(kind) == "tcp" && (port == 80 || port == s.App.ReservedTCPPort || (reality.Enabled && port == reality.Port))
			for _, p := range profiles {
				if p.Enabled && protocols.Transport(p.Kind) == protocols.Transport(kind) && p.Port == port {
					busy = true
				}
			}
			if !busy {
				break
			}
		}
		if port > 65535 {
			fail(w, 409, "Нет доступного порта в конфигурации")
			return
		}
		ports[kind] = port
	}
	respond(w, 200, map[string]any{"address": address, "address_source": source, "ports": ports, "port_verified": false, "vpn_verified": false})
}
