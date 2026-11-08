package store

import (
	"regexp"
	"strings"
)

const MaxLocalConnections = 32

var connectionIDPattern = regexp.MustCompile(`^vless-(reality|xhttp):[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// Existing protocol names remain stable aliases for the original listeners.
func ConnectionKind(id string) string {
	if connectionIDPattern.MatchString(id) {
		kind, _, _ := strings.Cut(id, ":")
		return kind
	}
	return id
}
func IsAdditionalConnection(id string) bool { return connectionIDPattern.MatchString(id) }
func (p Protocol) ConnectionID() string {
	if p.ID != "" {
		return p.ID
	}
	return p.Kind
}
