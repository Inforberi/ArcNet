package nodeprotocol

import "testing"

func TestValidateHTTPSOriginStrictly(t *testing.T) {
	for _, valid := range []string{"https://controller.example.com", "https://localhost:8443", "https://127.0.0.1:443", "https://[::1]:443", "https://example.com."} {
		if _, err := ValidateHTTPSOrigin(valid); err != nil {
			t.Errorf("rejected safe origin %q: %v", valid, err)
		}
	}
	for _, bad := range []string{
		"http://example.com", "https://", "https://a:b@example.com", "https://example.com/path",
		"https://example.com?q=1", "https://example.com?", "https://foo'bar.com",
		"https://foo;bar.com", "https://exa..mple.com", "https://-bad.com",
		"https://example.com-", "https://example.com:99999", "https://example.com:0",
		"https://example.com#frag", "https://example.com/",
	} {
		if _, err := ValidateHTTPSOrigin(bad); err == nil {
			t.Errorf("accepted unsafe origin %q", bad)
		}
	}
}
