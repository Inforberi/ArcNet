// Package releaseversion compares immutable release IDs. Legacy stage5-aN has
// numeric historical ordering; new releases use SemVer, with build ID separate.
package releaseversion

import (
	"regexp"
	"strings"
)

var legacy = regexp.MustCompile(`^0\.7\.0-stage5-a([1-9][0-9]{0,3})$`)
var core = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
var identifier = regexp.MustCompile(`^[0-9A-Za-z-]+$`)

type Version struct {
	Core   [3]string
	Pre    []string
	Legacy string
}

func numeric(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return s != ""
}
func Parse(raw string) (Version, bool) {
	var v Version
	if len(raw) < 5 || len(raw) > 64 {
		return v, false
	}
	parts := strings.Split(raw, "+")
	if len(parts) > 2 {
		return v, false
	}
	if len(parts) == 2 {
		for _, s := range strings.Split(parts[1], ".") {
			if !identifier.MatchString(s) {
				return v, false
			}
		}
	}
	main, pre, has := strings.Cut(parts[0], "-")
	m := core.FindStringSubmatch(main)
	if m == nil {
		return v, false
	}
	copy(v.Core[:], m[1:])
	if has {
		v.Pre = strings.Split(pre, ".")
		for _, s := range v.Pre {
			if !identifier.MatchString(s) || (numeric(s) && len(s) > 1 && s[0] == '0') {
				return Version{}, false
			}
		}
	}
	if m := legacy.FindStringSubmatch(parts[0]); m != nil {
		v.Legacy = m[1]
	}
	return v, true
}
func number(a, b string) int {
	if len(a) < len(b) {
		return -1
	}
	if len(a) > len(b) {
		return 1
	}
	return strings.Compare(a, b)
}
func Compare(a, b string) (int, bool) {
	x, ok := Parse(a)
	if !ok {
		return 0, false
	}
	y, ok := Parse(b)
	if !ok {
		return 0, false
	}
	for i := range x.Core {
		if n := number(x.Core[i], y.Core[i]); n != 0 {
			return n, true
		}
	}
	if x.Legacy != "" && y.Legacy != "" {
		return number(x.Legacy, y.Legacy), true
	}
	// All frozen historical A builds precede new releases of the same core.
	if x.Legacy != "" {
		return -1, true
	}
	if y.Legacy != "" {
		return 1, true
	}
	if len(x.Pre) == 0 && len(y.Pre) == 0 {
		return 0, true
	}
	if len(x.Pre) == 0 {
		return 1, true
	}
	if len(y.Pre) == 0 {
		return -1, true
	}
	for i := 0; i < len(x.Pre) && i < len(y.Pre); i++ {
		a, b := x.Pre[i], y.Pre[i]
		n := 0
		if numeric(a) && numeric(b) {
			n = number(a, b)
		} else if numeric(a) {
			n = -1
		} else if numeric(b) {
			n = 1
		} else {
			n = strings.Compare(a, b)
		}
		if n != 0 {
			return n, true
		}
	}
	if len(x.Pre) < len(y.Pre) {
		return -1, true
	}
	if len(x.Pre) > len(y.Pre) {
		return 1, true
	}
	return 0, true
}
func Legacy(raw string) bool      { return legacy.MatchString(raw) }
func Prerelease(raw string) bool  { v, ok := Parse(raw); return ok && len(v.Pre) > 0 && v.Legacy == "" }
func Publishable(raw string) bool { _, ok := Parse(raw); return ok && !strings.Contains(raw, "+") }
