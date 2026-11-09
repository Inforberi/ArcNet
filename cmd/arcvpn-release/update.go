package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"debug/buildinfo"
	"debug/elf"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/arcvpn/arcvpn/internal/releasebundle"
	"github.com/arcvpn/arcvpn/internal/releaseversion"
	"github.com/arcvpn/arcvpn/internal/updater"
)

func signingKey(privatePath, publicPath string) (ed25519.PrivateKey, ed25519.PublicKey, error) {
	key, err := updater.ReadPrivateKey(privatePath)
	if err != nil {
		return nil, nil, err
	}
	public, err := updater.ReadPublicKey(publicPath)
	if err != nil || !bytes.Equal(ed25519.PrivateKey(key).Public().(ed25519.PublicKey), public) {
		clear(key)
		return nil, nil, errors.New("ключ подписи не соответствует доверенному public key; ничего не опубликовано")
	}
	return key, public, nil
}

type buildReceipt struct {
	Format           string `json:"format"`
	Version          string `json:"version"`
	Target           string `json:"target"`
	CoreSHA256       string `json:"core_sha256"`
	FrontendVerified bool   `json:"frontend_verified"`
}

func validateReleaseBinary(data []byte, version string, metadata buildReceipt) error {
	image, err := elf.NewFile(bytes.NewReader(data))
	if err != nil || image.Machine != elf.EM_X86_64 {
		return errors.New("нужен Linux/amd64 ELF core")
	}
	info, err := buildinfo.Read(bytes.NewReader(data))
	if err != nil {
		return errors.New("нет Go build identity")
	}
	settings := map[string]string{}
	for _, s := range info.Settings {
		settings[s.Key] = s.Value
	}
	if settings["GOOS"] != "linux" || settings["GOARCH"] != "amd64" {
		return errors.New("неверная платформа core")
	}
	if metadata.Version != version || metadata.Format != "arcvpn-build-v1" || metadata.Target != "linux/amd64" || !metadata.FrontendVerified {
		return errors.New("build receipt не подтверждает выбранную версию и embedded frontend")
	}
	sum := sha256.Sum256(data)
	if metadata.CoreSHA256 != hex.EncodeToString(sum[:]) {
		return errors.New("core отличается от проверенного build receipt")
	}
	return nil
}
func prepareUpdate(args []string) error {
	flags := flag.NewFlagSet("prepare-update", flag.ContinueOnError)
	version := flags.String("version", "", "immutable release ID")
	minimum := flags.String("minimum-version", "", "oldest reviewed compatible release")
	binary := flags.String("binary", "", "verified Linux/amd64 core")
	receiptPath := flags.String("build-info", "", "receipt from verified release build")
	private := flags.String("key", "", "offline private key")
	public := flags.String("public-key", "deploy/update-public.hex", "independently trusted public key")
	source := flags.String("catalog-source", "", "catalog source JSON")
	notesPath := flags.String("notes", "", "release notes file")
	output := flags.String("out-dir", "", "new output directory")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || !releaseversion.Publishable(*version) || !releaseversion.Publishable(*minimum) || *output == "" || *notesPath == "" || *source == "" {
		return errors.New("prepare-update: version/minimum-version/binary/key/catalog-source/notes/out-dir обязательны")
	}
	key, pub, err := signingKey(*private, *public)
	if err != nil {
		return err
	}
	defer clear(key)
	data, err := releasebundle.ReadData(*binary, updater.MaxBinary)
	if err != nil {
		return err
	}
	receiptRaw, err := releasebundle.ReadData(*receiptPath, 64<<10)
	if err != nil {
		return err
	}
	var metadata buildReceipt
	if json.Unmarshal(receiptRaw, &metadata) != nil {
		return errors.New("invalid build receipt")
	}
	if err = validateReleaseBinary(data, *version, metadata); err != nil {
		return err
	}
	notes, err := releasebundle.ReadData(*notesPath, 2000)
	if err != nil || len(bytes.TrimSpace(notes)) == 0 {
		return errors.New("описание релиза: от 1 до 2000 байт")
	}
	raw, err := releasebundle.ReadData(*source, 64<<10)
	if err != nil {
		return err
	}
	// Validate existing source before extending it; reject unknown fields and versions.
	signed, err := updater.SignCatalog(raw, key)
	if err != nil {
		return err
	}
	catalog, err := updater.VerifyCatalog(signed, pub)
	if err != nil {
		return err
	}
	for _, r := range catalog.Releases {
		if r.Version == *version {
			return errors.New("идентификатор уже есть в каталоге; выберите новую версию")
		}
	}
	archive, err := updater.MakePackage(data, *version, key)
	if err != nil {
		return err
	}
	name := "update-" + *version + ".zip"
	legacy := releaseversion.Legacy(*version)
	if legacy {
		part := strings.LastIndex(*version, "-a")
		name = "update-a" + (*version)[part+2:] + ".zip"
	}
	sum := sha256.Sum256(archive)
	catalog.Signed = nil
	catalog.Partial = false
	if legacy {
		catalog.Format = "arcnet-update-catalog-v1"
	} else {
		catalog.Format = "arcnet-update-catalog-v2"
	}
	catalog.Releases = append(catalog.Releases, updater.Release{Version: *version, MinimumVersion: *minimum, Platform: "linux/amd64", Scope: "core-ui", File: name, Size: int64(len(archive)), SHA256: hex.EncodeToString(sum[:]), Notes: string(notes)})
	raw, err = json.MarshalIndent(catalog, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	signed, err = updater.SignCatalog(raw, key)
	if err != nil {
		return err
	}
	if _, err = updater.VerifyPublished(archive, signed, pub, *minimum, "linux/amd64"); err != nil {
		return err
	}
	files := map[string][]byte{name: archive, "catalog-source.json": raw, "release-notes.md": notes}
	catalogName := "update-catalog-v2.json"
	if legacy {
		catalogName = "update-catalog.json"
	}
	files[catalogName] = signed
	if err = os.Mkdir(*output, 0700); err != nil {
		return err
	}
	complete := false
	defer func() {
		if !complete {
			_ = os.RemoveAll(*output)
		}
	}()
	for name, data := range files {
		if err = releasebundle.WriteNew(filepath.Join(*output, name), data, 0600); err != nil {
			return err
		}
	}
	complete = true
	fmt.Println("Подписанный пакет и каталог проверены:", *output)
	return nil
}
