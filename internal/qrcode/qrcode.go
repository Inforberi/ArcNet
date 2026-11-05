// Package qrcode creates locally rendered SVG QR codes without external calls.
// libqrencode4 is a small native runtime dependency (also installed by install.sh).
package qrcode

/*
#cgo LDFLAGS: -l:libqrencode.so.4
#include <stdlib.h>
typedef struct QRcode {int version;int width;unsigned char *data;} QRcode;
extern QRcode* QRcode_encodeString8bit(const char *string, int version, int level);
extern void QRcode_free(QRcode *qrcode);
*/
import "C"
import (
	"errors"
	"fmt"
	"strings"
	"unsafe"
)

func SVG(value string) ([]byte, error) {
	if len(value) == 0 || len(value) > 2300 || strings.ContainsRune(value, 0) {
		return nil, errors.New("QR value is invalid or too large")
	}
	c := C.CString(value)
	defer C.free(unsafe.Pointer(c))
	code := C.QRcode_encodeString8bit(c, 0, 1)
	if code == nil {
		return nil, errors.New("QR encoding failed")
	}
	defer C.QRcode_free(code)
	width := int(code.width)
	if width <= 0 || width > 177 {
		return nil, errors.New("unexpected QR size")
	}
	cells := unsafe.Slice((*byte)(unsafe.Pointer(code.data)), width*width)
	size := width + 8
	var b strings.Builder
	b.Grow(1000 + width*width*10)
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" role="img" aria-label="QR-код"><rect width="%d" height="%d" fill="#fff"/><path fill="#111" d="`, size, size, size, size)
	for y := 0; y < width; y++ {
		for x := 0; x < width; x++ {
			if cells[y*width+x]&1 == 1 {
				fmt.Fprintf(&b, "M%d %dh1v1h-1z", x+4, y+4)
			}
		}
	}
	b.WriteString(`"/></svg>`)
	return []byte(b.String()), nil
}
