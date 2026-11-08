package backup

import (
	"github.com/arcvpn/arcvpn/internal/store"
	"reflect"
	"testing"
)

func TestUserPreferencesEncryptedRoundTrip(t *testing.T) {
	s := sample()
	s.Users[0].Preferences = store.UserPreferences{Version: 3, Group: "Team", Title: "Private", AllowedKinds: []string{}}
	blob, e := Encrypt(s, "correct-horse-battery-something")
	if e != nil {
		t.Fatal(e)
	}
	got, e := Decrypt(blob, "correct-horse-battery-something")
	if e != nil {
		t.Fatal(e)
	}
	_, users := got.ToStore()
	if !reflect.DeepEqual(users[0].Preferences, s.Users[0].Preferences) || users[0].Allows("wireguard") {
		t.Fatal("restore widened permissions")
	}
}
