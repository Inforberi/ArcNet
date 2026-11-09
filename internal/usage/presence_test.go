package usage

import "testing"

func TestPresenceRequiresFreshSessionEvidenceAndCompleteOfflineCoverage(t *testing.T) {
	var tracker PresenceTracker
	yes, no := true, false
	roster := []PresenceUser{{ID: 1, Identity: "one", Sources: []PresenceSource{{Source: "xray"}, {Source: "wireguard"}}}}
	tracker.Roster(roster)
	get := func(now int64) Presence {
		out, _ := tracker.Snapshot([]TrafficUser{{ID: 1, Identity: "one"}}, now)
		return out[1]
	}
	if get(100).State != "unknown" {
		t.Fatal("unobserved user is not offline")
	}
	tracker.Observe([]PresenceObservation{{UserID: 1, Identity: "one", PresenceSource: PresenceSource{Source: "xray", ObservedAt: 100, State: "online", Online: &yes}}}, 100)
	if p := get(100); p.State != "online" || p.CompleteCoverage {
		t.Fatal("partial online", p)
	}
	tracker.Observe([]PresenceObservation{{UserID: 1, Identity: "one", PresenceSource: PresenceSource{Source: "xray", ObservedAt: 101, State: "offline", Online: &no}}}, 101)
	if get(101).State != "unknown" {
		t.Fatal("WG cannot confirm global offline")
	}
	tracker.Observe([]PresenceObservation{{UserID: 1, Identity: "one", PresenceSource: PresenceSource{Source: "wireguard", ObservedAt: 102, State: "recent", LastHandshakeAt: 100}}}, 102)
	if get(102).State != "recent" {
		t.Fatal("handshake activity missing")
	}
	if get(130).State != "stale" {
		t.Fatal("old online must expire")
	}
	roster[0].Sources = roster[0].Sources[:1]
	tracker.Roster(roster)
	tracker.Observe([]PresenceObservation{{UserID: 1, Identity: "one", PresenceSource: PresenceSource{Source: "xray", ObservedAt: 131, State: "offline", Online: &no}}}, 131)
	if get(131).State != "offline" {
		t.Fatal("complete empty session source", get(131))
	}
	tracker.Observe([]PresenceObservation{{UserID: 1, Identity: "one", PresenceSource: PresenceSource{Source: "xray"}}}, 132)
	if get(132).State != "stale" {
		t.Fatal("failed source became offline")
	}
	roster[0].Identity = "replacement"
	tracker.Roster(roster)
	if get(133).State != "unknown" {
		t.Fatal("reused ID disclosed previous observations")
	}
}
