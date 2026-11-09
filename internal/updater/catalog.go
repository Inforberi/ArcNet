package updater

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/arcvpn/arcvpn/internal/publicnet"
	"github.com/arcvpn/arcvpn/internal/releaseversion"
)

const CatalogURL = "https://raw.githubusercontent.com/Inforberi/ArcNet/main/deploy/update-catalog.json"
const CatalogV2URL = "https://raw.githubusercontent.com/Inforberi/ArcNet/main/deploy/update-catalog-v2.json"
const catalogPrefix = "ArcNet update catalog v1\n"

var previewVersion = regexp.MustCompile(`^0\.7\.0-stage5-a([1-9][0-9]{0,3})$`)

type Release struct {
	Version        string `json:"version"`
	Platform       string `json:"platform"`
	MinimumVersion string `json:"minimum_version"`
	Scope          string `json:"scope"`
	File           string `json:"file"`
	Size           int64  `json:"size"`
	SHA256         string `json:"sha256"`
	Notes          string `json:"notes"`
	PublishedAt    string `json:"published_at,omitempty"`
}
type Catalog struct {
	Signed   []byte    `json:"-"`
	Format   string    `json:"format"`
	Releases []Release `json:"releases"`
	Partial  bool      `json:"-"`
}
type signedCatalog struct {
	Payload   string `json:"payload"`
	Signature string `json:"signature"`
}

func previewNumber(version string) int {
	m := previewVersion.FindStringSubmatch(version)
	if m == nil {
		return 0
	}
	n, _ := strconv.Atoi(m[1])
	return n
}
func decodeCatalog(raw []byte) (Catalog, error) {
	var c Catalog
	if len(raw) == 0 || len(raw) > 64<<10 {
		return c, ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&c) != nil || (c.Format != "arcnet-update-catalog-v1" && c.Format != "arcnet-update-catalog-v2") || len(c.Releases) > 32 {
		return c, ErrInvalid
	}
	if d.Decode(new(any)) != io.EOF {
		return c, ErrInvalid
	}
	seen := map[string]bool{}
	for _, r := range c.Releases {
		n := previewNumber(r.Version)
		min := previewNumber(r.MinimumVersion)
		comparison, valid := releaseversion.Compare(r.MinimumVersion, r.Version)
		file := "update-" + r.Version + ".zip"
		if n > 0 {
			file = "update-a" + strconv.Itoa(n) + ".zip"
		}
		if c.Format == "arcnet-update-catalog-v1" && (n < 5 || min < 5 || min > n || r.PublishedAt != "") {
			return c, ErrInvalid
		}
		if !valid || comparison > 0 || !releaseversion.Publishable(r.Version) || !releaseversion.Publishable(r.MinimumVersion) {
			return c, ErrInvalid
		}
		if r.PublishedAt != "" {
			if _, err := time.Parse(time.RFC3339, r.PublishedAt); err != nil {
				return c, ErrInvalid
			}
		}
		digest, err := hex.DecodeString(r.SHA256)
		if r.Platform != "linux/amd64" || r.Scope != "core-ui" || r.File != file || r.Size < 1 || r.Size > MaxArchive || err != nil || len(digest) != 32 || r.SHA256 != strings.ToLower(r.SHA256) || len(r.Notes) > 2000 || seen[r.Version] {
			return c, ErrInvalid
		}
		seen[r.Version] = true
	}
	sort.Slice(c.Releases, func(i, j int) bool {
		n, _ := releaseversion.Compare(c.Releases[i].Version, c.Releases[j].Version)
		return n > 0
	})
	return c, nil
}
func SignCatalog(raw []byte, key ed25519.PrivateKey) ([]byte, error) {
	if _, err := decodeCatalog(raw); err != nil || len(key) != ed25519.PrivateKeySize {
		return nil, ErrInvalid
	}
	signature := ed25519.Sign(key, append([]byte(catalogPrefix), raw...))
	return json.Marshal(signedCatalog{Payload: base64.StdEncoding.EncodeToString(raw), Signature: base64.StdEncoding.EncodeToString(signature)})
}
func VerifyCatalog(raw []byte, key ed25519.PublicKey) (Catalog, error) {
	var signed signedCatalog
	if len(raw) > 96<<10 || len(key) != ed25519.PublicKeySize {
		return Catalog{}, ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&signed) != nil || d.Decode(new(any)) != io.EOF {
		return Catalog{}, ErrInvalid
	}
	payload, e := base64.StdEncoding.DecodeString(signed.Payload)
	sig, e2 := base64.StdEncoding.DecodeString(signed.Signature)
	if e != nil || e2 != nil || !ed25519.Verify(key, append([]byte(catalogPrefix), payload...), sig) {
		return Catalog{}, ErrInvalid
	}
	c, err := decodeCatalog(payload)
	if err != nil {
		return Catalog{}, err
	}
	c.Signed = append([]byte(nil), raw...)
	return c, nil
}
func IsUpgrade(current, next string) bool {
	n, ok := releaseversion.Compare(current, next)
	return ok && n < 0 && (!releaseversion.Legacy(current) || previewNumber(current) >= 5)
}
func (r Release) Compatible(current, platform string) bool {
	n, ok := releaseversion.Compare(current, r.MinimumVersion)
	return ok && n >= 0 && r.Platform == platform && IsUpgrade(current, r.Version)
}
func (r Release) URL() string {
	return "https://github.com/Inforberi/ArcNet/releases/download/v" + r.Version + "/" + r.File
}
func FetchCatalog(ctx context.Context, key ed25519.PublicKey) (Catalog, error) {
	guard := publicnet.New()
	return fetchCatalog(ctx, key, guard.Fetch)
}
func fetchCatalog(ctx context.Context, key ed25519.PublicKey, fetch func(context.Context, string, int64) ([]byte, error)) (Catalog, error) {
	call, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	raw, err := fetch(call, CatalogV2URL, 96<<10)
	if err == nil {
		return VerifyCatalog(raw, key)
	} // A bad signature never falls back.
	raw, err = fetch(call, CatalogURL, 96<<10)
	if err != nil {
		return Catalog{}, err
	}
	c, err := VerifyCatalog(raw, key)
	c.Partial = true
	return c, err
}

// Downloads are fixed to release assets, public IPs and HTTPS; no environment proxy or arbitrary URL is accepted.
func Download(ctx context.Context, r Release, key ed25519.PublicKey) ([]byte, error) {
	guard := publicnet.New()
	transport := &http.Transport{Proxy: nil, DialContext: guard.DialContext, TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 10 * time.Second, MaxResponseHeaderBytes: 32 << 10, DisableCompression: true}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 90 * time.Second, CheckRedirect: releaseRedirect}
	req, err := http.NewRequestWithContext(ctx, "GET", r.URL(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 || resp.ContentLength > r.Size {
		return nil, ErrInvalid
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, r.Size+1))
	if err != nil {
		return nil, err
	}
	return verifyDownload(data, r, key)
}
func verifyDownload(data []byte, r Release, key ed25519.PublicKey) ([]byte, error) {
	sum := sha256.Sum256(data)
	if int64(len(data)) != r.Size || hex.EncodeToString(sum[:]) != r.SHA256 {
		return nil, ErrInvalid
	}
	payload, err := Verify(data, key)
	if err != nil || payload.Manifest.Version != r.Version || len(payload.Components) != 0 {
		return nil, ErrInvalid
	}
	return data, nil
}

func releaseRedirect(req *http.Request, via []*http.Request) error {
	host := req.URL.Hostname()
	if len(via) > 3 || !publicnet.ValidURL(req.URL.String()) || !(host == "github.com" || strings.HasSuffix(host, ".githubusercontent.com")) {
		return errors.New("untrusted release redirect")
	}
	return nil
}

func VerifyPublished(data, signed []byte, key ed25519.PublicKey, current, platform string) (Payload, error) {
	catalog, err := VerifyCatalog(signed, key)
	if err != nil {
		return Payload{}, err
	}
	payload, err := Verify(data, key)
	if err != nil || len(payload.Components) != 0 {
		return Payload{}, ErrInvalid
	}
	sum := sha256.Sum256(data)
	for _, r := range catalog.Releases {
		if r.Version == payload.Manifest.Version && r.Compatible(current, platform) && r.Size == int64(len(data)) && r.SHA256 == hex.EncodeToString(sum[:]) {
			return payload, nil
		}
	}
	return Payload{}, ErrInvalid
}
