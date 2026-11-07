package httpapi

import (
	"encoding/base64"
	"net/http"
	"strings"
	"unicode"
)

// Happ profile-title is UTF-8/base64 with a maximum of 25 displayed characters.
func subscriptionTitle(name string) string {
	name = strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, name))
	title := "ArcNet"
	if name != "" {
		title += " · " + name
	}
	runes := []rune(title)
	if len(runes) > 25 {
		runes = runes[:25]
	}
	return string(runes)
}
func setSubscriptionTitle(w http.ResponseWriter, name string) {
	w.Header().Set("Profile-Title", "base64:"+base64.StdEncoding.EncodeToString([]byte(subscriptionTitle(name))))
}
