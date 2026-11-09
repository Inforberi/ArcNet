// arcvpn-release is an offline build-host tool. It is not installed on a VPS.
package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/arcvpn/arcvpn/internal/releasebundle"
	"github.com/arcvpn/arcvpn/internal/releaseversion"
	"github.com/arcvpn/arcvpn/internal/updater"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "Ошибка:", err)
		os.Exit(1)
	}
}
func run(args []string) error {
	if len(args) == 0 {
		return errors.New("arcvpn-release: prepare-update | key-check | version-check | keygen | sign | verify | catalog-sign")
	}
	if args[0] == "prepare-update" {
		return prepareUpdate(args[1:])
	}
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	if args[0] == "version-check" {
		version := flags.String("version", "", "release ID")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if flags.NArg() != 0 || !releaseversion.Publishable(*version) {
			return errors.New("нужен уникальный SemVer без build metadata")
		}
		return nil
	}
	if args[0] == "key-check" {
		keyPath := flags.String("key", "", "private key")
		publicPath := flags.String("public-key", "deploy/update-public.hex", "trusted key")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		key, _, err := signingKey(*keyPath, *publicPath)
		clear(key)
		return err
	}
	if args[0] == "keygen" {
		dir := flags.String("out-dir", "", "private offline key directory")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if *dir == "" || flags.NArg() != 0 {
			return errors.New("keygen requires --out-dir")
		}
		if err := releasebundle.Keygen(*dir); err != nil {
			return err
		}
		fmt.Println("Signing key created outside installer payload; keep private key offline.")
		return nil
	}
	if args[0] == "catalog-sign" {
		input := flags.String("input", "", "catalog source JSON")
		keyPath := flags.String("key", "", "offline private key")
		output := flags.String("output", "", "signed catalog JSON")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if *input == "" || *keyPath == "" || *output == "" || flags.NArg() != 0 {
			return errors.New("catalog-sign requires --input --key --output")
		}
		key, err := releasebundle.ReadKey(*keyPath, true)
		if err != nil {
			return err
		}
		defer clear(key)
		raw, err := os.ReadFile(*input)
		if err != nil {
			return err
		}
		signed, err := updater.SignCatalog(raw, key)
		if err != nil {
			return err
		}
		return os.WriteFile(*output, signed, 0644)
	}
	if args[0] != "sign" && args[0] != "verify" {
		return errors.New("unknown release command")
	}
	archive := flags.String("archive", "", "installer tar.gz")
	manifest := flags.String("manifest", "", "signed manifest JSON")
	signature := flags.String("signature", "", "raw Ed25519 signature")
	key := flags.String("key", "", "offline private key for sign; independently trusted public key for verify")
	version := flags.String("version", "", "installer version (sign only)")
	source := flags.String("source-manifest", "", "source SHA256 manifest (sign only)")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 || *archive == "" || *manifest == "" || *signature == "" || *key == "" {
		return errors.New("provide --archive --manifest --signature --key")
	}
	if args[0] == "sign" {
		if *version == "" || *source == "" {
			return errors.New("sign requires --version and --source-manifest")
		}
		private, err := releasebundle.ReadKey(*key, true)
		if err != nil {
			return err
		}
		defer clear(private)
		data, err := releasebundle.ReadData(*source, 1<<20)
		if err != nil {
			return err
		}
		digest := sha256.Sum256(data)
		body, sig, err := releasebundle.Sign(*archive, *version, hex.EncodeToString(digest[:]), ed25519.PrivateKey(private))
		if err != nil {
			return err
		}
		if err := releasebundle.WriteNew(*manifest, body, 0644); err != nil {
			return err
		}
		if err := releasebundle.WriteNew(*signature, sig, 0644); err != nil {
			_ = os.Remove(*manifest)
			return err
		}
		fmt.Println("Signed installer:", filepath.Base(*archive))
		return nil
	}
	if *version != "" || *source != "" {
		return errors.New("verify does not accept signing metadata")
	}
	public, err := releasebundle.ReadKey(*key, false)
	if err != nil {
		return err
	}
	body, err := releasebundle.ReadData(*manifest, 4096)
	if err != nil {
		return err
	}
	sig, err := releasebundle.ReadData(*signature, 64)
	if err != nil {
		return err
	}
	m, err := releasebundle.Verify(*archive, body, sig, ed25519.PublicKey(public))
	if err != nil {
		return err
	}
	fmt.Printf("Verified installer: %s, platform=%s, signer-sha256=%s\n", m.File, m.Platform, releasebundle.Fingerprint(ed25519.PublicKey(public)))
	return nil
}
