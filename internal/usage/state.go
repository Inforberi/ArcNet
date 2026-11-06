package usage

import (
	"strconv"
	"time"
)

type Debit struct {
	UserID   int64 `json:"user_id"`
	Period   int64 `json:"period"`
	At       int64 `json:"at"`
	Upload   int64 `json:"upload_bytes"`
	Download int64 `json:"download_bytes"`
}
type State struct {
	Packages    []Package `json:"packages"`
	Checkpoints []Counter `json:"checkpoints"`
	Debits      []Debit   `json:"debits"`
}

func (s State) Validate(identities map[int64]string) error {
	if len(s.Packages) > 10000 || len(s.Checkpoints) > 40000 || len(s.Debits) > 65536 {
		return ErrInvalid
	}
	seen := map[int64]bool{}
	for _, p := range s.Packages {
		if p.Validate() != nil || p.ObservedAt > time.Now().Unix()+300 || identities[p.UserID] == "" || seen[p.UserID] {
			return ErrInvalid
		}
		seen[p.UserID] = true
	}
	keys := map[string]bool{}
	for _, c := range s.Checkpoints {
		k := strconv.FormatInt(c.UserID, 10) + "/" + c.StorageStream()
		if c.Validate() != nil || c.ObservedAt > time.Now().Unix()+300 || identities[c.UserID] != c.Identity || keys[k] {
			return ErrInvalid
		}
		keys[k] = true
	}
	keys = map[string]bool{}
	for _, d := range s.Debits {
		k := strconv.FormatInt(d.UserID, 10) + "/" + strconv.FormatInt(d.Period, 10) + "/" + strconv.FormatInt(d.At/60, 10)
		if identities[d.UserID] == "" || d.Period < 1 || d.At < 1 || d.Upload < 0 || d.Download < 0 || d.Upload > MaxBytes || d.Download > MaxBytes-d.Upload || keys[k] {
			return ErrInvalid
		}
		keys[k] = true
	}
	return nil
}
