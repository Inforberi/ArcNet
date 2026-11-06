package httpapi

import (
	"net/http"

	"github.com/arcvpn/arcvpn/internal/auth"
	"github.com/arcvpn/arcvpn/internal/qrcode"
)

func directLinkIDs(links []string) []string {
	ids := make([]string, 0, len(links))
	for _, link := range links {
		ids = append(ids, auth.HashToken(link))
	}
	return ids
}

// Selection is an opaque hash, not a credential in a URL. Re-check live links
// on every QR request: old selections cannot export revoked/changed credentials.
func writeSelectedAccessQR(w http.ResponseWriter, r *http.Request, links []string) {
	selection := r.URL.Query().Get("selection")
	if len(selection) != 64 {
		fail(w, 400, "Выберите подключение")
		return
	}
	for _, link := range links {
		if auth.HashToken(link) != selection {
			continue
		}
		image, e := qrcode.SVG(link)
		if e != nil {
			fail(w, 422, "Для этой ссылки используйте копирование")
			return
		}
		w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
		w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'; style-src 'none'")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(image)
		return
	}
	fail(w, 409, "Подключение изменилось или отозвано. Обновите список.")
}
func (s *Server) userDeviceQRCode(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	links, e := s.App.DeviceLinks(r.Context(), id, r.PathValue("device"))
	if e != nil {
		handleErr(w, e)
		return
	}
	writeSelectedAccessQR(w, r, links)
}
