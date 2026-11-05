// Package nodeprotocol defines the on-wire compatibility boundary between
// a controller and remote agents. Unrecognized protocol versions fail closed.
package nodeprotocol

const Current = 1

func Supports(v int) bool { return v == Current }
