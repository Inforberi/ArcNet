// Package releasebundle authenticates complete installer archives, including
// privileged scripts. It is deliberately separate from core-only updates.
package releasebundle

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"syscall"
)

const MaxArchive = 128 << 20
const SignatureDomain = "ArcVPN installer manifest v1\n"

var ErrInvalid = errors.New("invalid or untrusted ArcVPN installer bundle")
var versionPattern = regexp.MustCompile(`^0\.(6|7)\.(0|[1-9][0-9]{0,5})(-[A-Za-z0-9_-]+)?$`)
var digestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type Manifest struct {
	Format       string `json:"format"`
	Version      string `json:"version"`
	Platform     string `json:"platform"`
	File         string `json:"file"`
	Size         int64  `json:"size"`
	SHA256       string `json:"sha256"`
	SourceSHA256 string `json:"source_manifest_sha256"`
}

func (m Manifest) Validate() error {
	if m.Format != "arcvpn-installer-v1" || !versionPattern.MatchString(m.Version) || m.Platform != "linux/amd64" || m.File != "arcvpn-"+m.Version+"-linux-amd64.tar.gz" || m.Size <= 0 || m.Size > MaxArchive || !digestPattern.MatchString(m.SHA256) || !digestPattern.MatchString(m.SourceSHA256) {
		return ErrInvalid
	}
	return nil
}

// Open one no-follow descriptor; refuse FIFOs/devices before reading. Hashing
// is streamed and bounded, not an allocation of the entire archive.
func regularFile(path string, limit int64) (*os.File, error) {
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, ErrInvalid
	}
	f := os.NewFile(uintptr(fd), path)
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > limit {
		f.Close()
		return nil, ErrInvalid
	}
	return f, nil
}

func archiveDigest(path string) (int64, string, error) {
	f, err := regularFile(path, MaxArchive)
	if err != nil {
		return 0, "", err
	}
	defer f.Close()
	before, err := f.Stat()
	if err != nil {
		return 0, "", ErrInvalid
	}
	hash := sha256.New()
	size, err := io.Copy(hash, io.LimitReader(f, MaxArchive+1))
	after, statErr := f.Stat()
	if err != nil || statErr != nil || size != before.Size() || size > MaxArchive || after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) {
		return 0, "", ErrInvalid
	}
	return size, hex.EncodeToString(hash.Sum(nil)), nil
}

func Sign(archive, version, sourceSHA string, private ed25519.PrivateKey) ([]byte, []byte, error) {
	if len(private) != ed25519.PrivateKeySize {
		return nil, nil, ErrInvalid
	}
	size, digest, err := archiveDigest(archive)
	if err != nil {
		return nil, nil, err
	}
	manifest := Manifest{Format: "arcvpn-installer-v1", Version: version, Platform: "linux/amd64", File: filepath.Base(archive), Size: size, SHA256: digest, SourceSHA256: sourceSHA}
	if err := manifest.Validate(); err != nil {
		return nil, nil, err
	}
	body, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	body = append(body, '\n')
	signature := ed25519.Sign(private, append([]byte(SignatureDomain), body...))
	return body, signature, nil
}

func Verify(archive string, body, signature []byte, public ed25519.PublicKey) (Manifest, error) {
	var manifest Manifest
	if len(body) == 0 || len(body) > 4096 || len(signature) != ed25519.SignatureSize || len(public) != ed25519.PublicKeySize || !ed25519.Verify(public, append([]byte(SignatureDomain), body...), signature) {
		return manifest, ErrInvalid
	}
	if json.Unmarshal(body, &manifest) != nil || manifest.Validate() != nil || filepath.Base(archive) != manifest.File {
		return Manifest{}, ErrInvalid
	}
	size, digest, err := archiveDigest(archive)
	if err != nil || size != manifest.Size || digest != manifest.SHA256 {
		return Manifest{}, ErrInvalid
	}
	return manifest, nil
}
