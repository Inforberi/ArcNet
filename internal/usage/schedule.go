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
}

func (s ResetSchedule) location() (*time.Location, error) {
	if len(s.Timezone) < 1 || len(s.Timezone) > 64 || s.Timezone == "Local" {
		return nil, ErrInvalid
	}
	return time.LoadLocation(s.Timezone)
}
func (s ResetSchedule) Next(now time.Time) (int64, error) {
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
		days := (8 - int(now.Weekday())) % 7
		if days == 0 {
			days = 7
		}
		next = time.Date(now.Year(), now.Month(), now.Day()+days, 0, 0, 0, 0, loc)
	case "monthly":
		next = time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, loc)
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
