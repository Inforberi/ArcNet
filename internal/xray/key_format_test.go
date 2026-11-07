package xray

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateKeysAcceptsXrayPublicKeyLabels(t *testing.T) {
	for _, label := range []string{"Password", "Password (PublicKey)", "PublicKey", "Public key"} {
		t.Run(label, func(t *testing.T) {
			binary := filepath.Join(t.TempDir(), "xray")
			private, public := strings.Repeat("A", 43), strings.Repeat("B", 43)
			body := "#!/bin/sh\nprintf '%s\\n' 'PrivateKey: " + private + "' '" + label + ": " + public + "' 'Hash32: " + strings.Repeat("C", 43) + "'\n"
			if err := os.WriteFile(binary, []byte(body), 0700); err != nil {
				t.Fatal(err)
			}
			gotPrivate, gotPublic, id, err := GenerateKeys(context.Background(), binary)
			if err != nil || gotPrivate != private || gotPublic != public || !hexID.MatchString(id) {
				t.Fatal("key format rejected or Hash32 used", err)
			}
		})
	}
}
