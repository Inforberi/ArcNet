// Package usage owns monotonic traffic accounting, independent of OS metrics.
package usage

import (
	"errors"
	"fmt"
)

const MaxBytes int64 = 9_000_000_000_000_000 // Exact in SQLite and JavaScript.
const FreshFor = 20

var ErrInvalid = errors.New("invalid traffic observation")

type Counter struct {
	UserID     int64  `json:"user_id"`
	Identity   string `json:"identity"`
	Stream     string `json:"stream"`
	Epoch      string `json:"epoch"`
	StartedAt  int64  `json:"started_at"`
	ObservedAt int64  `json:"observed_at"`
	Upload     int64  `json:"upload"`
	Download   int64  `json:"download"`
}

func (c Counter) Validate() error {
	if c.UserID < 1 || len(c.Identity) != 36 || (c.Stream != "vless-reality" && c.Stream != "vless-xhttp") || len(c.Epoch) < 1 || len(c.Epoch) > 128 || c.StartedAt < 1 || c.ObservedAt < 1 || c.Upload < 0 || c.Download < 0 || c.Upload > MaxBytes || c.Download > MaxBytes-c.Upload {
		return ErrInvalid
	}
	return nil
}

// Delta never resets an upstream counter. A new, later process epoch starts at
// zero; old/repeated observations and regressions cannot debit twice.
func Delta(previous *Counter, current Counter) (int64, int64, error) {
	if err := current.Validate(); err != nil {
		return 0, 0, err
	}
	if previous == nil {
		return current.Upload, current.Download, nil
	}
	if current.UserID != previous.UserID || current.Stream != previous.Stream {
		return 0, 0, ErrInvalid
	}
	if current.Identity != previous.Identity {
		return 0, 0, fmt.Errorf("%w: credential changed", ErrInvalid)
	}
	if current.ObservedAt < previous.ObservedAt || (current.ObservedAt == previous.ObservedAt && current.Epoch == previous.Epoch) {
		return 0, 0, nil
	}
	if current.StartedAt < previous.StartedAt {
		return 0, 0, fmt.Errorf("%w: retired epoch", ErrInvalid)
	}
	if current.Epoch != previous.Epoch {
		if current.StartedAt <= previous.StartedAt {
			return 0, 0, ErrInvalid
		}
		return current.Upload, current.Download, nil
	}
	if current.StartedAt != previous.StartedAt || current.Upload < previous.Upload || current.Download < previous.Download {
		return 0, 0, fmt.Errorf("%w: counter regressed", ErrInvalid)
	}
	return current.Upload - previous.Upload, current.Download - previous.Download, nil
}

type Package struct {
	UserID          int64 `json:"user_id"`
	QuotaBytes      int64 `json:"quota_bytes"`
	Upload          int64 `json:"upload_bytes"`
	Download        int64 `json:"download_bytes"`
	ExpiresAt       int64 `json:"expires_at"`
	Period          int64 `json:"period"`
	PeriodStartedAt int64 `json:"period_started_at"`
	Version         int64 `json:"version"`
	ObservedAt      int64 `json:"observed_at"`
	Incomplete      bool  `json:"incomplete"`
	NeedsReview     bool  `json:"needs_review"`
}

func (p Package) Validate() error {
	if p.UserID < 1 || p.QuotaBytes < 0 || p.QuotaBytes > MaxBytes || p.Upload < 0 || p.Download < 0 || p.Upload > MaxBytes || p.Download > MaxBytes-p.Upload || p.ExpiresAt < 0 || p.ExpiresAt > 253402300799 || p.Period < 1 || p.Period >= MaxBytes || p.PeriodStartedAt < 0 || p.Version < 0 || p.Version >= MaxBytes || p.ObservedAt < 0 {
		return ErrInvalid
	}
	return nil
}
func (p Package) Reason(now int64, sourceFresh bool) string {
	if p.ExpiresAt > 0 && now >= p.ExpiresAt {
		return "expired"
	}
	if p.QuotaBytes > 0 && p.Upload+p.Download >= p.QuotaBytes {
		return "quota_exhausted"
	}
	if p.QuotaBytes > 0 && p.NeedsReview {
		return "accounting_gap"
	}
	if p.QuotaBytes > 0 && !sourceFresh {
		return "accounting_unavailable"
	}
	return ""
}
