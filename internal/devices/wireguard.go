package devices

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"net/netip"
)

func WireGuardAddress(address string) bool {
	ip, err := netip.ParseAddr(address)
	if err != nil || !ip.Is4() {
		return false
	}
	b := ip.As4()
	n := int(b[2])*256 + int(b[3])
	return b[0] == 10 && b[1] == 76 && n >= 2 && n <= 65534
}
func NewWireGuardKeys() (string, string, error) {
	key, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}
	return base64.StdEncoding.EncodeToString(key.Bytes()), base64.StdEncoding.EncodeToString(key.PublicKey().Bytes()), nil
}
func ValidateWireGuard(d Device) error {
	if d.WGPrivate == "" && d.WGPublic == "" && d.WGAddress == "" {
		return nil
	}
	secret, err := base64.StdEncoding.DecodeString(d.WGPrivate)
	if err != nil {
		return ErrInvalid
	}
	key, err := ecdh.X25519().NewPrivateKey(secret)
	if err != nil || base64.StdEncoding.EncodeToString(key.PublicKey().Bytes()) != d.WGPublic || !WireGuardAddress(d.WGAddress) {
		return ErrInvalid
	}
	return nil
}
