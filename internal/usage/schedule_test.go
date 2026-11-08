package usage

import (
	"testing"
	"time"
)

func TestResetScheduleCalendar(t *testing.T) {
	tests := []struct{ frequency, zone, now, want string }{
		{"daily", "America/New_York", "2026-03-08T00:00:00-05:00", "2026-03-09T00:00:00-04:00"},
		{"daily", "America/New_York", "2026-11-01T00:00:00-04:00", "2026-11-02T00:00:00-05:00"},
		{"weekly", "Europe/Moscow", "2026-11-08T13:00:00+03:00", "2026-11-09T00:00:00+03:00"},
		{"weekly", "Europe/Moscow", "2026-11-09T00:00:00+03:00", "2026-11-16T00:00:00+03:00"},
		{"monthly", "UTC", "2028-02-29T23:59:00Z", "2028-03-01T00:00:00Z"},
	}
	for _, tt := range tests {
		now, _ := time.Parse(time.RFC3339, tt.now)
		want, _ := time.Parse(time.RFC3339, tt.want)
		got, err := (ResetSchedule{Frequency: tt.frequency, Timezone: tt.zone}).Next(now)
		if err != nil || got != want.Unix() {
			t.Fatal(tt, got, err)
		}
	}
	for _, s := range []ResetSchedule{{Frequency: "hourly", Timezone: "UTC"}, {Frequency: "daily", Timezone: "Local"}, {Frequency: "daily", Timezone: "Unknown/Zone"}} {
		if _, err := s.Next(time.Now()); err == nil {
			t.Fatal("invalid schedule", s)
		}
	}
}
