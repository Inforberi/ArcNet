package releasebundle

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func ReadKey(path string, private bool) ([]byte, error) {
	f, err := regularFile(path, 256)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || (private && info.Mode().Perm()&0077 != 0) {
		return nil, ErrInvalid
	}
	data, err := io.ReadAll(io.LimitReader(f, 257))
	if err != nil {
		return nil, ErrInvalid
	}
	key, err := hex.DecodeString(strings.TrimSpace(string(data)))
	length := ed25519.PublicKeySize
	if private {
		length = ed25519.PrivateKeySize
	}
	if err != nil || len(key) != length {
		return nil, ErrInvalid
	}
	return key, nil
}

func ReadData(path string, limit int64) ([]byte, error) {
	f, err := regularFile(path, limit)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil || int64(len(data)) > limit {
		return nil, ErrInvalid
	}
	return data, nil
}

func WriteNew(path string, data []byte, mode os.FileMode) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	_, writeErr := f.Write(data)
	if writeErr == nil {
		writeErr = f.Sync()
	}
	closeErr := f.Close()
	if writeErr != nil {
		_ = os.Remove(path)
		return writeErr
	}
	if closeErr != nil {
		_ = os.Remove(path)
		return closeErr
	}
	return nil
}

func Keygen(dir string) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return ErrInvalid
	}
	names := []string{"installer-private.hex", "installer-public.hex", "installer-public.pem"}
	for _, name := range names {
		if _, err := os.Lstat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			return ErrInvalid
		}
	}
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	defer clear(private)
	der, err := x509.MarshalPKIXPublicKey(public)
	if err != nil {
		return err
	}
	data := [][]byte{[]byte(hex.EncodeToString(private) + "\n"), []byte(hex.EncodeToString(public) + "\n"), pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})}
	created := []string{}
	for i, name := range names {
		path := filepath.Join(dir, name)
		mode := os.FileMode(0644)
		if i == 0 {
			mode = 0600
		}
		if err := WriteNew(path, data[i], mode); err != nil {
			for _, path := range created {
				_ = os.Remove(path)
			}
			return err
		}
		created = append(created, path)
	}
	return nil
}

func Fingerprint(public ed25519.PublicKey) string {
	digest := sha256.Sum256(public)
	return hex.EncodeToString(digest[:])
}
