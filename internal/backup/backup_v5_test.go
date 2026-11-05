package backup

import (
	"reflect"
	"testing"

	"github.com/arcvpn/arcvpn/internal/routingprofile"
)

func TestV5PreservesRoutingProfileIntentAndRejectsLegacyAmbiguity(t *testing.T) {
	p := routingprofile.Default()
	p.ID = "01234567-89ab-4cde-8f01-23456789abcd"
	p.Name = "Мои правила"
	p.Version = 3
	p.Enabled = true
	p.Direct = []string{"domain:example.com"}
	snapshot := sample().WithRoutingProfiles([]routingprofile.Profile{p})
	snapshot.Version = 5
	blob, err := Encrypt(snapshot, "backup-password-long-enough")
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decrypt(blob, "backup-password-long-enough")
	if err != nil || got.Version != 5 || !reflect.DeepEqual(got.RoutingProfiles, snapshot.RoutingProfiles) {
		t.Fatalf("profile lost: %#v %v", got.RoutingProfiles, err)
	}
	snapshot.Version = 4
	if err := snapshot.Validate(); err == nil {
		t.Fatal("legacy archive silently accepted new field")
	}
	snapshot.Version = 5
	snapshot.RoutingProfiles[0].Version = 0
	if err := snapshot.Validate(); err == nil {
		t.Fatal("invalid profile revision accepted")
	}
}
