package backup

import (
	"github.com/arcvpn/arcvpn/internal/usage"
	"testing"
	"time"
)

func TestScheduledPackageBackupRoundTrip(t *testing.T) {
	s := sample()
	s.Usage = &usage.State{Packages: []usage.Package{{UserID: s.Users[0].ID, Period: 1, ResetSchedule: &usage.ResetSchedule{Frequency: "monthly", Timezone: "Europe/Moscow", NextAt: time.Now().Add(24 * time.Hour).Unix()}}}}
	blob, err := Encrypt(s, "correct-horse-battery-something")
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decrypt(blob, "correct-horse-battery-something")
	if err != nil || got.Usage.Packages[0].ResetSchedule.Timezone != "Europe/Moscow" {
		t.Fatal(err)
	}
	s.Version = 14
	if s.Validate() == nil {
		t.Fatal("old format accepted schedule")
	}
}
