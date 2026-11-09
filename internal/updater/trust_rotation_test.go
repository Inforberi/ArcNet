package updater

import (
	"os"
	"testing"
)

func TestExplicitTrustRotationDoesNotAcceptLegacySigner(t *testing.T) {
	current, err := ReadPublicKey("../../deploy/update-public.hex")
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := ReadPublicKey("../../deploy/trust/legacy-2026-11-09/update-public.hex")
	if err != nil {
		t.Fatal(err)
	}
	oldCatalog, err := os.ReadFile("../../deploy/trust/legacy-2026-11-09/update-catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = VerifyCatalog(oldCatalog, legacy); err != nil {
		t.Fatal("archived legacy catalog damaged", err)
	}
	if _, err = VerifyCatalog(oldCatalog, current); err == nil {
		t.Fatal("old signer silently trusted after rotation")
	}
	for _, name := range []string{"../../deploy/update-catalog.json", "../../deploy/update-catalog-v2.json"} {
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = VerifyCatalog(raw, current); err != nil {
			t.Fatal("current catalog signer mismatch", err)
		}
		if _, err = VerifyCatalog(raw, legacy); err == nil {
			t.Fatal("old installation silently trusts new signer")
		}
	}
}
