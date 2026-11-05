// Package routingprofile owns editable traffic-selection profiles. Saving or
// exporting a profile never activates a server route or publishes subscriptions.
package routingprofile

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"
)

const MaxProfiles = 32
const MaxEntries = 2048
const MaxImportBytes = 256 << 10

var ErrInvalid = errors.New("invalid routing profile")
var labelPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
var categoryPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_@!-]{0,79}$`)

// Lists are evaluated in Order, followed by DefaultAction. Tokens within each
// list use explicit Xray domain/full/geoip/geosite prefixes or IP/CIDR syntax.
type Profile struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Enabled        bool     `json:"enabled"`
	DefaultAction  string   `json:"default_action"`
	Order          []string `json:"order"`
	Proxy          []string `json:"proxy"`
	Direct         []string `json:"direct"`
	Block          []string `json:"block"`
	DomainStrategy string   `json:"domain_strategy"`
	RemoteDNS      string   `json:"remote_dns"`
	DirectDNS      string   `json:"direct_dns"`
	Version        int64    `json:"version"`
	UpdatedAt      int64    `json:"updated_at"`
}
type Bundle struct {
	Format  string  `json:"format"`
	Version int     `json:"version"`
	Profile Profile `json:"profile"`
}
type Decision struct {
	Action          string   `json:"action"`
	Matched         string   `json:"matched,omitempty"`
	Status          string   `json:"status"` // model | needs_geodata | needs_dns
	Reason          string   `json:"reason"`
	Pending         []string `json:"pending"`
	NetworkVerified bool     `json:"network_verified"`
}

func Default() Profile {
	return Profile{DefaultAction: "proxy", Order: []string{"block", "proxy", "direct"}, Proxy: []string{}, Direct: []string{}, Block: []string{}, DomainStrategy: "AsIs", RemoteDNS: "https://cloudflare-dns.com/dns-query", DirectDNS: "https://dns.google/dns-query"}
}
func Hostname(s string) bool {
	if len(s) < 3 || len(s) > 253 || !strings.Contains(s, ".") {
		return false
	}
	if _, err := netip.ParseAddr(s); err == nil {
		return false
	}
	for _, label := range strings.Split(s, ".") {
		if !labelPattern.MatchString(label) {
			return false
		}
	}
	return true
}
func Token(raw string) (string, error) {
	s := strings.ToLower(strings.TrimSpace(raw))
	if len(s) == 0 || len(s) > 260 || strings.ContainsAny(s, "\x00\r\n\t ") {
		return "", fmt.Errorf("%w: пустая или некорректная запись", ErrInvalid)
	}
	if ip, err := netip.ParseAddr(s); err == nil {
		if ip.Is4In6() {
			ip = ip.Unmap()
		}
		return ip.String(), nil
	}
	if prefix, err := netip.ParsePrefix(s); err == nil {
		if prefix.Addr().Is4In6() {
			return "", fmt.Errorf("%w: используйте обычный IPv4 CIDR", ErrInvalid)
		}
		return prefix.Masked().String(), nil
	}
	kind, value, ok := strings.Cut(s, ":")
	if !ok {
		kind = "domain"
		value = s
	}
	switch kind {
	case "domain", "full":
		if Hostname(value) {
			return kind + ":" + value, nil
		}
	case "geosite", "geoip":
		if categoryPattern.MatchString(value) {
			return kind + ":" + value, nil
		}
	}
	return "", fmt.Errorf("%w: ожидается домен, IP/CIDR или категория geosite/geoip", ErrInvalid)
}
func ParseList(text string) ([]string, int, error) {
	if len(text) > MaxImportBytes {
		return nil, 0, fmt.Errorf("%w: список превышает 256 КБ", ErrInvalid)
	}
	items := []string{}
	seen := map[string]bool{}
	duplicates := 0
	for index, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		token, err := Token(line)
		if err != nil {
			return nil, 0, fmt.Errorf("строка %d: %w", index+1, err)
		}
		if seen[token] {
			duplicates++
			continue
		}
		seen[token] = true
		items = append(items, token)
		if len(items) > MaxEntries {
			return nil, 0, fmt.Errorf("%w: не более %d записей", ErrInvalid, MaxEntries)
		}
	}
	return items, duplicates, nil
}
func validDNS(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.User == nil && u.Fragment == "" && u.RawQuery == "" && u.Port() == "" && Hostname(u.Hostname()) && len(raw) <= 512 && u.Opaque == "" && !strings.ContainsAny(raw, "\x00\r\n\\")
}
func Normalize(p Profile) (Profile, error) {
	p.Name = strings.TrimSpace(p.Name)
	if !utf8.ValidString(p.Name) || len([]rune(p.Name)) < 1 || len([]rune(p.Name)) > 80 || strings.ContainsAny(p.Name, "\x00\r\n") {
		return Profile{}, fmt.Errorf("%w: название от 1 до 80 символов", ErrInvalid)
	}
	if p.DefaultAction != "proxy" && p.DefaultAction != "direct" && p.DefaultAction != "block" {
		return Profile{}, fmt.Errorf("%w: неизвестное действие по умолчанию", ErrInvalid)
	}
	seenOrder := map[string]bool{}
	if len(p.Order) != 3 {
		return Profile{}, fmt.Errorf("%w: задайте три группы правил", ErrInvalid)
	}
	for _, action := range p.Order {
		if seenOrder[action] || (action != "proxy" && action != "direct" && action != "block") {
			return Profile{}, fmt.Errorf("%w: некорректный порядок", ErrInvalid)
		}
		seenOrder[action] = true
	}
	if p.DomainStrategy != "AsIs" && p.DomainStrategy != "IPIfNonMatch" && p.DomainStrategy != "IPOnDemand" {
		return Profile{}, fmt.Errorf("%w: некорректная DNS-стратегия", ErrInvalid)
	}
	if !validDNS(p.RemoteDNS) || !validDNS(p.DirectDNS) {
		return Profile{}, fmt.Errorf("%w: DNS должен быть HTTPS URL с доменным именем без логина, порта, query и fragment", ErrInvalid)
	}
	seen := map[string]string{}
	count := 0
	for _, entry := range []struct {
		action string
		list   *[]string
	}{{"proxy", &p.Proxy}, {"direct", &p.Direct}, {"block", &p.Block}} {
		normalized := []string{}
		for _, raw := range *entry.list {
			value, err := Token(raw)
			if err != nil {
				return Profile{}, err
			}
			if previous, exists := seen[value]; exists {
				return Profile{}, fmt.Errorf("%w: запись %s повторяется в %s/%s", ErrInvalid, value, previous, entry.action)
			}
			seen[value] = entry.action
			normalized = append(normalized, value)
			count++
			if count > MaxEntries {
				return Profile{}, fmt.Errorf("%w: суммарно не более %d записей", ErrInvalid, MaxEntries)
			}
		}
		*entry.list = normalized
	}
	p.Order = append([]string{}, p.Order...)
	blob, err := json.Marshal(p)
	if err != nil || len(blob) > 64<<10 {
		return Profile{}, fmt.Errorf("%w: профиль превышает 64 КБ", ErrInvalid)
	}
	return p, nil
}
func (p Profile) List(action string) []string {
	switch action {
	case "proxy":
		return p.Proxy
	case "direct":
		return p.Direct
	default:
		return p.Block
	}
}
func Decode(blob []byte) (Profile, error) {
	if len(blob) > MaxImportBytes {
		return Profile{}, fmt.Errorf("%w: файл превышает 256 КБ", ErrInvalid)
	}
	var bundle Bundle
	dec := json.NewDecoder(strings.NewReader(string(blob)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&bundle); err != nil {
		return Profile{}, fmt.Errorf("%w: ожидается JSON-профиль ArcVPN", ErrInvalid)
	}
	var tail any
	if dec.Decode(&tail) != io.EOF {
		return Profile{}, fmt.Errorf("%w: лишние данные после JSON", ErrInvalid)
	}
	if bundle.Format != "arcvpn-routing-profile" || bundle.Version != 1 {
		return Profile{}, fmt.Errorf("%w: неподдерживаемый формат", ErrInvalid)
	}
	bundle.Profile.ID = ""
	bundle.Profile.Version = 0
	bundle.Profile.UpdatedAt = 0
	return Normalize(bundle.Profile)
}

// Explain never resolves network destinations or assumes bundled geodata exists.
// An earlier unresolved condition prevents claiming a later rule is effective.
func Explain(p Profile, address string) (Decision, error) {
	p, err := Normalize(p)
	if err != nil {
		return Decision{}, err
	}
	input := strings.ToLower(strings.TrimSpace(address))
	ip, ipErr := netip.ParseAddr(input)
	if ipErr != nil && !Hostname(input) {
		return Decision{}, fmt.Errorf("%w: введите домен или IP без URL", ErrInvalid)
	}
	result := Decision{Action: "proxy", Status: "model", Reason: "Дополнительные правила выключены: используется базовый VPN-маршрут", Pending: []string{}}
	if !p.Enabled {
		return result, nil
	}
	for _, action := range p.Order {
		for _, token := range p.List(action) {
			kind, value, _ := strings.Cut(token, ":")
			match := false
			pending := ""
			switch {
			case strings.HasPrefix(token, "geosite:"):
				if ipErr != nil {
					pending = token
				}
			case strings.HasPrefix(token, "geoip:"):
				pending = token
			case kind == "domain":
				match = ipErr != nil && (input == value || strings.HasSuffix(input, "."+value))
			case kind == "full":
				match = ipErr != nil && input == value
			default:
				if prefix, e := netip.ParsePrefix(token); e == nil {
					if ipErr == nil {
						match = prefix.Contains(ip.Unmap())
					} else if p.DomainStrategy != "AsIs" {
						pending = token
					}
				}
				if target, e := netip.ParseAddr(token); e == nil {
					if ipErr == nil {
						match = target.Unmap() == ip.Unmap()
					} else if p.DomainStrategy != "AsIs" {
						pending = token
					}
				}
			}
			if pending != "" {
				result.Pending = append(result.Pending, pending)
			}
			if match {
				if len(result.Pending) > 0 {
					return unresolved(result), nil
				}
				result.Action = action
				result.Matched = token
				result.Reason = "Совпало правило профиля"
				return result, nil
			}
		}
	}
	if len(result.Pending) > 0 {
		return unresolved(result), nil
	}
	result.Action = p.DefaultAction
	result.Reason = "Совпадений нет: действие для остального трафика"
	return result, nil
}
func unresolved(result Decision) Decision {
	result.Action = ""
	result.Status = "needs_dns_or_geodata"
	result.Reason = "Предыдущее правило требует DNS/геобазы клиента; достоверный выбор выхода пока невозможен"
	return result
}
