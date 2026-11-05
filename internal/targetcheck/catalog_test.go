package targetcheck

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"
)

func TestSignedCatalogRequiresIndependentSignatureExpiryAndStrictJSON(t *testing.T) {
	key, private, _ := ed25519.GenerateKey(rand.Reader)
	encode := func(c Catalog) []byte {
		payload, _ := json.Marshal(c)
		signature := ed25519.Sign(private, append([]byte("arcvpn-target-catalog-v1\x00"), payload...))
		blob, _ := json.Marshal(SignedCatalog{Payload: base64.StdEncoding.EncodeToString(payload), Signature: base64.StdEncoding.EncodeToString(signature)})
		return blob
	}
	c := DefaultCatalog()
	c.Version = 7
	c.ExpiresAt = time.Now().Add(time.Hour).Unix()
	blob := encode(c)
	got, err := VerifyCatalog(blob, key)
	if err != nil || got.FeedVersion != 7 {
		t.Fatalf("valid signature rejected: %#v %v", got, err)
	}
	wrong, _, _ := ed25519.GenerateKey(rand.Reader)
	if _, err := VerifyCatalog(blob, wrong); err == nil {
		t.Fatal("foreign signer accepted")
	}
	if _, err := VerifyCatalog(append(blob, []byte(" {}")...), key); err == nil {
		t.Fatal("trailing data accepted")
	}
	c.ExpiresAt = time.Now().Add(-time.Second).Unix()
	if _, err := VerifyCatalog(encode(c), key); err == nil {
		t.Fatal("expired catalogue accepted")
	}
	c.ExpiresAt = time.Now().Add(time.Hour).Unix()
	c.FeedVersion = 9
	if _, err := VerifyCatalog(encode(c), key); err == nil {
		t.Fatal("untrusted local revision accepted")
	}
}
