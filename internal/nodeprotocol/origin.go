package nodeprotocol

import (
	"errors"
	"net/netip"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var dnsLabel = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?$`)

// ValidateHTTPSOrigin is shared by the privileged bootstrap and the agent.
// Reject shell-significant host strings as well as malformed DNS labels: the
// controller origin can also appear in operator-facing CLI commands.
func ValidateHTTPSOrigin(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Path != "" || u.Opaque != "" {
		return "", errors.New("controller must be a valid HTTPS origin")
	}
	host := u.Hostname()
	if host == "" {
		return "", errors.New("controller hostname is missing")
	}
	port := u.Port()
	if port != "" {
		value, err := strconv.Atoi(port)
		if err != nil || value < 1 || value > 65535 {
			return "", errors.New("invalid controller HTTPS port")
		}
	}
	if _, err := netip.ParseAddr(host); err != nil {
		normalized := strings.TrimSuffix(host, ".")
		if len(normalized) == 0 || len(normalized) > 253 {
			return "", errors.New("invalid controller DNS hostname")
		}
		for _, label := range strings.Split(normalized, ".") {
			if !dnsLabel.MatchString(label) {
				return "", errors.New("invalid controller DNS label")
			}
		}
	}
	return strings.TrimRight(u.String(), "/"), nil
}
