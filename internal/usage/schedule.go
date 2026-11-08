package usage

import (
	"time"
	_ "time/tzdata"
)

// Calendar boundaries use the selected IANA zone; expiry is independent.
type ResetSchedule struct {
	Frequency string `json:"frequency"`
	Timezone  string `json:"timezone"`
	NextAt    int64  `json:"next_at"`
	Weekday   *int   `json:"weekday,omitempty"`
	Day       int    `json:"day,omitempty"`
}

func (s ResetSchedule) location() (*time.Location, error) {
	if len(s.Timezone) < 1 || len(s.Timezone) > 64 || s.Timezone == "Local" {
		return nil, ErrInvalid
	}
	return time.LoadLocation(s.Timezone)
}
func (s ResetSchedule) Next(now time.Time) (int64, error) {
	if s.Day < 0 || s.Day > 31 || (s.Weekday != nil && (*s.Weekday < 0 || *s.Weekday > 6)) || (s.Frequency != "weekly" && s.Weekday != nil) || (s.Frequency != "monthly" && s.Day != 0) {
		return 0, ErrInvalid
	}
	loc, err := s.location()
	if err != nil {
		return 0, ErrInvalid
	}
	now = now.In(loc)
	var next time.Time
	switch s.Frequency {
	case "daily":
		next = time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, loc)
	case "weekly":
		weekday := 1
		if s.Weekday != nil {
			weekday = *s.Weekday
		}
		days := (weekday - int(now.Weekday()) + 7) % 7
		if days == 0 {
			days = 7
		}
		next = time.Date(now.Year(), now.Month(), now.Day()+days, 0, 0, 0, 0, loc)
	case "monthly":
		day := s.Day
		if day == 0 {
			day = 1
		}
		boundary := func(month time.Month) time.Time {
			last := time.Date(now.Year(), month+1, 0, 0, 0, 0, 0, loc).Day()
			return time.Date(now.Year(), month, min(day, last), 0, 0, 0, 0, loc)
		}
		next = boundary(now.Month())
		if !next.After(now) {
			next = boundary(now.Month() + 1)
		}
	default:
		return 0, ErrInvalid
	}
	if next.Unix() <= now.Unix() || next.Unix() > 253402300799 {
		return 0, ErrInvalid
	}
	return next.Unix(), nil
}
func (s ResetSchedule) Valid() bool {
	_, err := s.Next(time.Unix(1700000000, 0))
	return err == nil && s.NextAt > 0 && s.NextAt <= 253402300799
}
