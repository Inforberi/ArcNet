// Package updater verifies operator-signed ArcVPN application upgrades. HTTPS
// transport and a checksum *inside* an archive are not sufficient to authorize
// replacement of a privileged server binary.
package updater

import (
	"archive/zip"
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
)

const MaxArchive = 128 << 20
const MaxBinary = 52 << 20
const PublicKeyFile = "/etc/arcvpn/update-public.hex"
const StagedArchive = "/var/lib/arcvpn/update/staged.zip"
const CoreBinary = "/opt/arcvpn/bin/arcvpn-core"

type Artifact struct {
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}
type Manifest struct {
	Version    string              `json:"version"`
	SHA256     string              `json:"sha256"`
	Size       int64               `json:"size"`
	Components map[string]Artifact `json:"components,omitempty"`
}
type Payload struct {
	Manifest   Manifest
	Binary     []byte
	Components map[string][]byte
}

var ErrInvalid = errors.New("invalid or untrusted ArcVPN update package")

// readTrustKey uses a single non-following descriptor and a small input limit.
// Public keys may be world-readable; private signing keys must be owner-only.
func readTrustKey(path string, private bool) ([]byte, error) {
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, ErrInvalid
	}
	f := os.NewFile(uintptr(fd), path)
	defer f.Close()
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() || st.Size() == 0 || st.Size() > 256 || (private && st.Mode().Perm()&0077 != 0) {
		return nil, ErrInvalid
	}
	data, err := io.ReadAll(io.LimitReader(f, 257))
	if err != nil || len(data) == 0 || len(data) > 256 {
		return nil, ErrInvalid
	}
	return data, nil
}

func ReadPublicKey(path string) (ed25519.PublicKey, error) {
	data, err := readTrustKey(path, false)
	if err != nil {
		return nil, err
	}
	raw, err := hex.DecodeString(strings.TrimSpace(string(data)))
	if err != nil || len(raw) != ed25519.PublicKeySize {
		return nil, ErrInvalid
	}
	return ed25519.PublicKey(raw), nil
}

var allowedComponents = map[string]bool{
	"xray": true, "hysteria": true, "awg": true, "awg-quick": true, "amneziawg-go": true,
}

func validComponentSet(names map[string]bool) bool {
	count := 0
	for _, name := range []string{"awg", "awg-quick", "amneziawg-go"} {
		if names[name] {
			count++
		}
	}
	return count == 0 || count == 3
}

func Verify(blob []byte, public ed25519.PublicKey) (Payload, error) {
	if len(blob) == 0 || len(blob) > MaxArchive || len(public) != ed25519.PublicKeySize {
		return Payload{}, ErrInvalid
	}
	zr, err := zip.NewReader(bytes.NewReader(blob), int64(len(blob)))
	if err != nil {
		return Payload{}, ErrInvalid
	}
	files := map[string][]byte{}
	var total int64
	for _, file := range zr.File {
		name := file.Name
		max := int64(4096)
		if name == "arcvpn-core" || (strings.HasPrefix(name, "components/") && allowedComponents[strings.TrimPrefix(name, "components/")]) {
			max = MaxBinary
		} else if name != "manifest.json" && name != "manifest.sig" {
			return Payload{}, ErrInvalid
		}
		if _, exists := files[name]; exists || file.FileInfo().IsDir() || file.FileInfo().Mode()&os.ModeSymlink != 0 || file.UncompressedSize64 > uint64(max) {
			return Payload{}, ErrInvalid
		}
		total += int64(file.UncompressedSize64)
		if total > MaxArchive {
			return Payload{}, ErrInvalid
		}
		reader, err := file.Open()
		if err != nil {
			return Payload{}, ErrInvalid
		}
		b, err := io.ReadAll(io.LimitReader(reader, max+1))
		closeErr := reader.Close()
		if err != nil || closeErr != nil || int64(len(b)) > max || len(b) == 0 {
			return Payload{}, ErrInvalid
		}
		files[name] = b
	}
	for _, name := range []string{"manifest.json", "manifest.sig", "arcvpn-core"} {
		if len(files[name]) == 0 {
			return Payload{}, ErrInvalid
		}
	}
	rawSig, err := hex.DecodeString(strings.TrimSpace(string(files["manifest.sig"])))
	if err != nil || !ed25519.Verify(public, files["manifest.json"], rawSig) {
		return Payload{}, ErrInvalid
	}
	var manifest Manifest
	dec := json.NewDecoder(bytes.NewReader(files["manifest.json"]))
	dec.DisallowUnknownFields()
	if dec.Decode(&manifest) != nil || manifest.Size != int64(len(files["arcvpn-core"])) || manifest.Size < 1000 || manifest.Size > MaxBinary || !validVersion(manifest.Version) {
		return Payload{}, ErrInvalid
	}
	digest := sha256.Sum256(files["arcvpn-core"])
	if hex.EncodeToString(digest[:]) != manifest.SHA256 {
		return Payload{}, ErrInvalid
	}
	if len(files) != len(manifest.Components)+3 {
		return Payload{}, ErrInvalid
	}
	componentNames := make(map[string]bool, len(manifest.Components))
	for name := range manifest.Components {
		componentNames[name] = true
	}
	if !validComponentSet(componentNames) {
		return Payload{}, ErrInvalid
	}
	out := Payload{Manifest: manifest, Binary: files["arcvpn-core"], Components: make(map[string][]byte, len(manifest.Components))}
	for name, meta := range manifest.Components {
		if !allowedComponents[name] || meta.Size < 1 || meta.Size > MaxBinary {
			return Payload{}, ErrInvalid
		}
		b := files["components/"+name]
		if len(b) == 0 || int64(len(b)) != meta.Size {
			return Payload{}, ErrInvalid
		}
		hash := sha256.Sum256(b)
		if hex.EncodeToString(hash[:]) != meta.SHA256 {
			return Payload{}, ErrInvalid
		}
		out.Components[name] = b
	}
	return out, nil
}
func validVersion(v string) bool {
	if len(v) < 3 || len(v) > 64 {
		return false
	}
	for _, r := range v {
		if !(r >= '0' && r <= '9' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r == '.' || r == '-' || r == '_') {
			return false
		}
	}
	return true
}
func MakePackage(binary []byte, version string, private ed25519.PrivateKey) ([]byte, error) {
	return MakePackageWithComponents(binary, version, private, nil)
}
func MakePackageWithComponents(binary []byte, version string, private ed25519.PrivateKey, extras map[string][]byte) ([]byte, error) {
	if len(private) != ed25519.PrivateKeySize || len(binary) < 1000 || len(binary) > MaxBinary || !validVersion(version) {
		return nil, ErrInvalid
	}
	total := int64(len(binary))
	hash := sha256.Sum256(binary)
	manifest := Manifest{Version: version, SHA256: hex.EncodeToString(hash[:]), Size: int64(len(binary))}
	if len(extras) > 0 {
		manifest.Components = make(map[string]Artifact, len(extras))
	}
	names := make([]string, 0, len(extras))
	componentNames := make(map[string]bool, len(extras))
	for name := range extras {
		componentNames[name] = true
	}
	if !validComponentSet(componentNames) {
		return nil, ErrInvalid
	}
	for name, data := range extras {
		total += int64(len(data))
		if !allowedComponents[name] || len(data) == 0 || len(data) > MaxBinary || total > MaxArchive {
			return nil, ErrInvalid
		}
		sum := sha256.Sum256(data)
		manifest.Components[name] = Artifact{Size: int64(len(data)), SHA256: hex.EncodeToString(sum[:])}
		names = append(names, name)
	}
	sort.Strings(names)
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		return nil, err
	}
	signature := ed25519.Sign(private, manifestBytes)
	out := &bytes.Buffer{}
	writer := zip.NewWriter(out)
	contents := []struct {
		name string
		body []byte
	}{{"manifest.json", manifestBytes}, {"manifest.sig", []byte(hex.EncodeToString(signature))}, {"arcvpn-core", binary}}
	for _, name := range names {
		contents = append(contents, struct {
			name string
			body []byte
		}{"components/" + name, extras[name]})
	}
	for _, item := range contents {
		file, err := writer.Create(item.name)
		if err != nil {
			return nil, err
		}
		if _, err = file.Write(item.body); err != nil {
			return nil, err
		}
	}
	if err = writer.Close(); err != nil {
		return nil, err
	}
	if out.Len() > MaxArchive {
		return nil, ErrInvalid
	}
	return out.Bytes(), nil
}
func GenerateKeyPair(dir string) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	files := []struct {
		name     string
		contents []byte
		mode     os.FileMode
	}{
		{"update-private.hex", []byte(hex.EncodeToString(priv) + "\n"), 0600},
		{"update-public.hex", []byte(hex.EncodeToString(pub) + "\n"), 0644},
	}
	for _, f := range files {
		p := filepath.Join(dir, f.name)
		file, e := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, f.mode)
		if e != nil {
			return fmt.Errorf("create %s: %w", f.name, e)
		}
		_, e = file.Write(f.contents)
		closeErr := file.Close()
		if e != nil {
			return e
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}
func ReadPrivateKey(path string) (ed25519.PrivateKey, error) {
	data, err := readTrustKey(path, true)
	if err != nil {
		return nil, err
	}
	raw, err := hex.DecodeString(strings.TrimSpace(string(data)))
	if err != nil || len(raw) != ed25519.PrivateKeySize {
		return nil, ErrInvalid
	}
	return ed25519.PrivateKey(raw), nil
}
