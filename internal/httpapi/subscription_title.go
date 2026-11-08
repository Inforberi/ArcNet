package httpapi

import (
	"encoding/base64"
	"github.com/arcvpn/arcvpn/internal/store"
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

func userSubscriptionTitle(u store.User) string {
	if u.Preferences.Title == "" {
		return subscriptionTitle(u.Name)
	}
	r := []rune(u.Preferences.Title)
	if len(r) > 25 {
		r = r[:25]
	}
	return string(r)
}
func setUserSubscriptionTitle(w http.ResponseWriter, u store.User) {
	w.Header().Set("Profile-Title", "base64:"+base64.StdEncoding.EncodeToString([]byte(userSubscriptionTitle(u))))
}
