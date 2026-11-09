package updater

import (
	"context"
	"encoding/json"
	"github.com/arcvpn/arcvpn/internal/publicnet"
	"github.com/arcvpn/arcvpn/internal/releaseversion"
	"strings"
	"time"
)

// Informational metadata only. It has no artifact URLs or installation authority.
type PublicRelease struct {
	Version     string `json:"version"`
	Notes       string `json:"notes"`
	PublishedAt string `json:"published_at,omitempty"`
}

func parseHistory(raw []byte) ([]PublicRelease, error) {
	var rows []struct {
		Tag       string `json:"tag_name"`
		Body      string `json:"body"`
		Published string `json:"published_at"`
		Draft     bool   `json:"draft"`
	}
	if len(raw) > 256<<10 || json.Unmarshal(raw, &rows) != nil || len(rows) > 30 {
		return nil, ErrInvalid
	}
	out := []PublicRelease{}
	seen := map[string]bool{}
	for _, r := range rows {
		id := strings.TrimPrefix(r.Tag, "v")
		if r.Draft || !releaseversion.Publishable(id) || seen[id] {
			continue
		}
		seen[id] = true
		date := ""
		if _, err := time.Parse(time.RFC3339, r.Published); err == nil {
			date = r.Published
		}
		notes := []rune(r.Body)
		if len(notes) > 2000 {
			notes = notes[:2000]
		}
		out = append(out, PublicRelease{Version: id, Notes: string(notes), PublishedAt: date})
	}
	return out, nil
}
func FetchReleaseHistory(ctx context.Context) ([]PublicRelease, error) {
	data, err := publicnet.New().Fetch(ctx, "https://api.github.com/repos/Inforberi/ArcNet/releases?per_page=20", 256<<10)
	if err != nil {
		return nil, err
	}
	return parseHistory(data)
}
