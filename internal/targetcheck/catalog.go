package targetcheck

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"
)

const MaxCatalogBytes = 64 << 10
const MaxCatalogCandidates = 64

type Catalog struct {
	Version     int64       `json:"version"`
	Candidates  []Candidate `json:"candidates"`
	Source      string      `json:"source"`
	FeedVersion int64       `json:"feed_version,omitempty"`
	ExpiresAt   int64       `json:"expires_at,omitempty"`
}
type SignedCatalog struct {
	Payload   string `json:"payload"`
	Signature string `json:"signature"`
}

func DefaultCatalog() Catalog {
	// Candidates, not certified recommendations. No node inherits them as an
	// active setting. Each actual target must pass this node's bounded checks.
	return Catalog{Version: 1, Source: "bundled-candidates", Candidates: []Candidate{{Host: "dl.google.com", SNI: "dl.google.com"}, {Host: "www.microsoft.com", SNI: "www.microsoft.com"}, {Host: "www.apple.com", SNI: "www.apple.com"}}}
}
func ValidateCatalog(c Catalog) (Catalog, error) {
	if c.Version < 1 || c.FeedVersion < 0 || c.ExpiresAt < 0 || len(c.Candidates) < 1 || len(c.Candidates) > MaxCatalogCandidates {
		return Catalog{}, errors.New("каталог должен иметь версию и от 1 до 64 кандидатов")
	}
	normalized := []Candidate{}
	seen := map[string]bool{}
	for _, item := range c.Candidates {
		items, err := Normalize([]Candidate{item})
		if err != nil {
			return Catalog{}, err
		}
		key := items[0].Host + "/" + items[0].SNI
		if seen[key] {
			return Catalog{}, errors.New("каталог содержит повторяющиеся цели")
		}
		seen[key] = true
		normalized = append(normalized, items[0])
	}
	c.Candidates = normalized
	if len(c.Source) > 2048 || strings.ContainsAny(c.Source, "\r\n\x00") {
		return Catalog{}, errors.New("некорректный источник каталога")
	}
	return c, nil
}
func ParseCandidates(text string) ([]Candidate, error) {
	if len(text) > MaxCatalogBytes {
		return nil, errors.New("каталог превышает 64 КБ")
	}
	candidates := []Candidate{}
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		host, sni, found := strings.Cut(line, ",")
		if !found {
			sni = host
		}
		candidates = append(candidates, Candidate{Host: host, SNI: sni})
	}
	c, err := ValidateCatalog(Catalog{Version: 1, Candidates: candidates, Source: "manual"})
	return c.Candidates, err
}
func VerifyCatalog(blob []byte, key ed25519.PublicKey) (Catalog, error) {
	if len(blob) > MaxCatalogBytes*2 || len(key) != ed25519.PublicKeySize {
		return Catalog{}, errors.New("не задан независимый ключ каталога или превышен размер")
	}
	var signed SignedCatalog
	if err := strictJSON(blob, &signed); err != nil {
		return Catalog{}, errors.New("некорректный signed catalog")
	}
	payload, err := base64.StdEncoding.DecodeString(signed.Payload)
	if err != nil || len(payload) > MaxCatalogBytes {
		return Catalog{}, errors.New("некорректный payload каталога")
	}
	signature, err := base64.StdEncoding.DecodeString(signed.Signature)
	if err != nil || !ed25519.Verify(key, append([]byte("arcvpn-target-catalog-v1\x00"), payload...), signature) {
		return Catalog{}, errors.New("подпись каталога не прошла проверку")
	}
	var c Catalog
	if err := strictJSON(payload, &c); err != nil {
		return Catalog{}, errors.New("некорректные данные каталога")
	}
	if c.FeedVersion != 0 || c.ExpiresAt <= time.Now().Unix() {
		return Catalog{}, errors.New("подписанный каталог устарел или содержит некорректную версию источника")
	}
	c.FeedVersion = c.Version
	return ValidateCatalog(c)
}
func strictJSON(blob []byte, out any) error {
	dec := json.NewDecoder(bytes.NewReader(blob))
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return err
	}
	var tail any
	if dec.Decode(&tail) != io.EOF {
		return errors.New("unexpected JSON tail")
	}
	return nil
}
