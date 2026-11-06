package nodeprobe

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/netip"

	"github.com/arcvpn/arcvpn/internal/nodebootstrap"
)

const CascadeDNSMode = "doh_via_cascade"

// RunCascade uses the same authenticated SOCKS/VLESS client as Run. All probe
// HTTP/TLS and DoH traffic uses that transport; there is no direct fallback.
// It does not measure an arbitrary application's OS DNS leak behaviour.
func RunCascade(ctx context.Context, binary, link string) (Result, error) {
	return run(ctx, binary, link, checkCascade)
}
func checkCascade(ctx context.Context, client *http.Client) (Result, error) {
	request, e := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.ipify.org?format=json", nil)
	if e != nil {
		return Result{}, e
	}
	response, e := client.Do(request)
	if e != nil {
		return Result{}, errors.New("cascade egress HTTPS failed")
	}
	data, e := io.ReadAll(io.LimitReader(response.Body, 1025))
	response.Body.Close()
	if e != nil || len(data) > 1024 || response.StatusCode != 200 {
		return Result{}, errors.New("invalid egress response")
	}
	var body struct {
		IP string `json:"ip"`
	}
	if json.Unmarshal(data, &body) != nil {
		return Result{}, errors.New("invalid egress IP")
	}
	ip, e := netip.ParseAddr(body.IP)
	if e != nil || !nodebootstrap.PublicIP(ip) {
		return Result{}, errors.New("egress IP is not public")
	}
	// RFC1035 wireformat: fixed query, no operator URL/domain injection.
	query := []byte{0, 0, 1, 0, 0, 1, 0, 0, 0, 0, 0, 0, 7, 'e', 'x', 'a', 'm', 'p', 'l', 'e', 3, 'c', 'o', 'm', 0, 0, 1, 0, 1}
	if _, e := rand.Read(query[:2]); e != nil {
		return Result{}, e
	}
	request, e = http.NewRequestWithContext(ctx, http.MethodPost, "https://1.1.1.1/dns-query", bytes.NewReader(query))
	if e != nil {
		return Result{}, e
	}
	request.Header.Set("Content-Type", "application/dns-message")
	request.Header.Set("Accept", "application/dns-message")
	response, e = client.Do(request)
	if e != nil {
		return Result{}, errors.New("cascade DNS over HTTPS failed")
	}
	data, e = io.ReadAll(io.LimitReader(response.Body, 4097))
	response.Body.Close()
	if e != nil || len(data) > 4096 || response.StatusCode != 200 || response.Header.Get("Content-Type") != "application/dns-message" || !validDNSAnswer(query, data) {
		return Result{}, errors.New("invalid DNS roundtrip through cascade")
	}
	return Result{Result: "pass", ExitIP: ip.Unmap().String(), DNSMode: CascadeDNSMode, Evidence: "verified HTTPS egress and fixed DoH wire query through VLESS cascade; not a device DNS leak test"}, nil
}
func validDNSAnswer(query, answer []byte) bool {
	if len(answer) < len(query) || !bytes.Equal(query[:2], answer[:2]) || answer[2]&0x80 == 0 || answer[2]&0x7a != 0 || answer[3]&0x0f != 0 || binary.BigEndian.Uint16(answer[4:6]) != 1 || !bytes.Equal(query[12:], answer[12:len(query)]) {
		return false
	}
	count := int(binary.BigEndian.Uint16(answer[6:8]))
	if count < 1 || count > 64 {
		return false
	}
	at := len(query)
	found := false
	for i := 0; i < count; i++ {
		for {
			if at >= len(answer) {
				return false
			}
			size := int(answer[at])
			at++
			if size&0xc0 == 0xc0 {
				if at >= len(answer) || ((size&0x3f)<<8|int(answer[at])) >= len(answer) {
					return false
				}
				at++
				break
			}
			if size == 0 {
				break
			}
			if size > 63 || at+size > len(answer) {
				return false
			}
			at += size
		}
		if at+10 > len(answer) {
			return false
		}
		kind := binary.BigEndian.Uint16(answer[at:])
		class := binary.BigEndian.Uint16(answer[at+2:])
		size := int(binary.BigEndian.Uint16(answer[at+8:]))
		at += 10
		if at+size > len(answer) {
			return false
		}
		if kind == 1 && class == 1 && size == 4 {
			var value [4]byte
			copy(value[:], answer[at:at+4])
			if nodebootstrap.PublicIP(netip.AddrFrom4(value)) {
				found = true
			}
		}
		at += size
	}
	return found
}
