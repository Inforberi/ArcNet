package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"os"
	"testing"

	"github.com/arcvpn/arcvpn/internal/protocols"
)

func TestKeyRotationPreviewCASAndIdentity(t *testing.T) {
	a := firstProtocolApp(t)
	ctx := context.Background()
	p, err := a.AddProtocol(ctx, protocols.WireGuard, "198.51.100.1", 0, "", "")
	if err != nil {
		t.Fatal(err)
	}
	u, err := a.AddUser(ctx, "Alice")
	if err != nil {
		t.Fatal(err)
	}
	creds, _ := a.Store.CredentialsForUser(ctx, u.ID)
	plan, err := a.PreviewKeyRotation(ctx, p.Kind)
	if err != nil || plan.Affected != 1 || len(plan.Users) != 1 {
		t.Fatal("preview", err)
	}
	if _, err = a.AddUser(ctx, "Bob"); err != nil {
		t.Fatal(err)
	}
	if err = a.RotateConnectionKeys(ctx, p.Kind, plan.Revision, plan.Confirmation); !errors.Is(err, ErrConnectionChanged) {
		t.Fatal("changed audience accepted", err)
	}
	plan, err = a.PreviewKeyRotation(ctx, p.Kind)
	if err != nil {
		t.Fatal(err)
	}
	if err = a.RotateConnectionKeys(ctx, p.Kind, plan.Revision, plan.Confirmation); err != nil {
		t.Fatal(err)
	}
	got, err := a.Store.GetProtocol(ctx, p.Kind)
	if err != nil || got.PrivateKey == p.PrivateKey || got.PublicKey == p.PublicKey {
		t.Fatal("keys did not rotate", err)
	}
	after, _ := a.Store.ByID(ctx, u.ID)
	current, _ := a.Store.CredentialsForUser(ctx, u.ID)
	if after.UUID != u.UUID || after.Subscription != u.Subscription || len(current) != len(creds) || current[0] != creds[0] {
		t.Fatal("user identity changed")
	}
	if err = a.RotateConnectionKeys(ctx, p.Kind, plan.Revision, plan.Confirmation); !errors.Is(err, ErrConnectionChanged) {
		t.Fatal("rotation replay accepted", err)
	}
	if a.PublicationVersion()%2 != 0 {
		t.Fatal("publication remained locked")
	}
}

func TestKeyRotationRealityAndXHTTP(t *testing.T) {
	for _, kind := range []string{"vless-reality", protocols.XHTTP} {
		t.Run(kind, func(t *testing.T) {
			a := firstProtocolApp(t)
			ctx := context.Background()
			if kind == "vless-reality" {
				if err := a.Setup(ctx, "198.51.100.1", "www.example.com", 443); err != nil {
					t.Fatal(err)
				}
			} else {
				if _, err := a.AddProtocol(ctx, kind, "198.51.100.1", 2087, "www.example.com", "www.example.com"); err != nil {
					t.Fatal(err)
				}
			}
			plan, old, err := a.rotationPlanLocked(ctx, kind)
			if err != nil {
				t.Fatal(err)
			}
			priv, pub, err := protocols.WGKeys()
			if err != nil {
				t.Fatal(err)
			}
			private, _ := base64.StdEncoding.DecodeString(priv)
			public, _ := base64.StdEncoding.DecodeString(pub)
			script := "#!/bin/sh\nif [ \"$1\" = x25519 ]; then\necho 'PrivateKey: " + base64.RawURLEncoding.EncodeToString(private) + "'\necho 'Password (PublicKey): " + base64.RawURLEncoding.EncodeToString(public) + "'\nfi\nexit 0\n"
			if err = os.WriteFile(a.Core.Binary, []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			if err = a.RotateConnectionKeys(ctx, kind, plan.Revision, plan.Confirmation); err != nil {
				t.Fatal(err)
			}
			_, got, err := a.rotationPlanLocked(ctx, kind)
			if err != nil || got.PrivateKey == old.PrivateKey || got.PublicKey == old.PublicKey || got.ShortID == old.ShortID {
				t.Fatal("Reality keypair/short ID not rotated", err)
			}
		})
	}
}
func TestKeyRotationRollback(t *testing.T) {
	a := firstProtocolApp(t)
	ctx := context.Background()
	if err := a.Setup(ctx, "198.51.100.1", "www.example.com", 443); err != nil {
		t.Fatal(err)
	}
	p, err := a.AddProtocol(ctx, protocols.WireGuard, "198.51.100.1", 0, "", "")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := a.PreviewKeyRotation(ctx, p.Kind)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(a.Core.Path)
	if err = os.WriteFile(a.Core.Binary, []byte("#!/bin/sh\nif test ! -f \"$0.failed\"; then touch \"$0.failed\";exit 1;fi\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if err = a.RotateConnectionKeys(ctx, p.Kind, plan.Revision, plan.Confirmation); err == nil {
		t.Fatal("failed runtime accepted")
	}
	got, err := a.Store.GetProtocol(ctx, p.Kind)
	if err != nil || got.PrivateKey != p.PrivateKey || got.PublicKey != p.PublicKey {
		t.Fatal("rollback lost old keys", err)
	}
	if a.PublicationVersion()%2 != 0 {
		t.Fatal("failed rotation left publication locked")
	}
	runtime, _ := os.ReadFile(a.Core.Path)
	if !bytes.Equal(before, runtime) {
		t.Fatal("runtime not restored")
	}
}
