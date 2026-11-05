package qrcode

import (
	"bytes"
	"encoding/xml"
	"testing"
)

func TestSVG(t *testing.T) {
	content, err := SVG("https://127.0.0.1:8443/sub/0123456789abcdefghijk")
	if err != nil {
		t.Fatal(err)
	}
	var v any
	_ = v
	var element struct {
		XMLName xml.Name `xml:"svg"`
	}
	if err = xml.Unmarshal(content, &element); err != nil {
		t.Fatalf("invalid SVG: %v", err)
	}
	if !bytes.Contains(content, []byte("<path")) {
		t.Fatal("QR pattern absent")
	}
	if _, err = SVG(""); err == nil {
		t.Fatal("empty payload accepted")
	}
	if _, err = SVG(string(bytes.Repeat([]byte{'x'}, 2400))); err == nil {
		t.Fatal("oversized payload accepted")
	}
}
