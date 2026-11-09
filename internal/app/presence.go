package app

import (
	"context"
	"github.com/arcvpn/arcvpn/internal/usage"
	"time"
)

func (a *App) cacheWireGuardPresence(counters []usage.Counter) {
	a.wireGuardPresence = nil
	for _, c := range counters {
		a.wireGuardPresence = append(a.wireGuardPresence, usage.PresenceObservation{UserID: c.UserID, Identity: c.Identity, PresenceSource: usage.PresenceSource{Source: "wireguard", State: "fresh", Epoch: c.Epoch, ObservedAt: c.ObservedAt, LastHandshakeAt: c.LastHandshakeAt}})
	}
}
func (a *App) collectPresenceLocked(ctx context.Context) {
	users, err := a.Store.ListUsers(ctx)
	if err != nil {
		return
	}
	users, err = a.xrayUsers(ctx, users)
	if err != nil {
		return
	}
	profiles, err := a.Store.ListProtocols(ctx)
	if err != nil {
		return
	}
	reality, err := a.Store.OptionalReality(ctx)
	if err != nil {
		return
	}
	if reality.Address != "" {
		profiles = append(profiles, RealityProfile(reality))
	}
	at := time.Now().Unix()
	trafficUsers := make([]usage.TrafficUser, 0, len(users))
	for _, u := range users {
		trafficUsers = append(trafficUsers, usage.TrafficUser{ID: u.ID, Identity: u.UUID})
	}
	traffic, _ := a.UserTraffic.Snapshot(at, trafficUsers)
	activity := map[int64]int64{}
	for _, row := range traffic {
		activity[row.UserID] = row.LastActivityAt
	}
	remoteSources, remoteErr := a.Store.NodePresenceSources(ctx)
	roster := make([]usage.PresenceUser, 0, len(users))
	required := map[string]bool{}
	for _, u := range users {
		owner := usage.PresenceUser{ID: u.ID, Identity: u.UUID, LastActivityAt: activity[u.ID]}
		xrayConnections := []string{}
		for _, p := range profiles {
			if !p.Enabled || !u.Allows(p.ConnectionID()) {
				continue
			}
			if p.Kind == "vless-reality" || p.Kind == "vless-xhttp" {
				xrayConnections = append(xrayConnections, p.ConnectionID())
				continue
			}
			owner.Sources = append(owner.Sources, usage.PresenceSource{Source: p.Kind, ConnectionID: p.ConnectionID()})
			required[p.Kind] = true
		}
		if len(xrayConnections) > 0 {
			source := usage.PresenceSource{Source: "xray"}
			if len(xrayConnections) == 1 {
				source.ConnectionID = xrayConnections[0]
			}
			owner.Sources = append(owner.Sources, source)
			required["xray"] = true
		}
		owner.Sources = append(owner.Sources, remoteSources...)
		if remoteErr != nil || a.runtimeUncertain || a.usageApplied == "" {
			owner.Sources = append(owner.Sources, usage.PresenceSource{Source: "unmonitored"})
		}
		roster = append(roster, owner)
	}
	a.UserPresence.Roster(roster)
	observe := func(kind string, rows []usage.PresenceObservation, err error) {
		if err == nil {
			a.UserPresence.Observe(rows, time.Now().Unix())
			return
		}
		failed := make([]usage.PresenceObservation, 0, len(users))
		for _, u := range users {
			failed = append(failed, usage.PresenceObservation{UserID: u.ID, Identity: u.UUID, PresenceSource: usage.PresenceSource{Source: kind}})
		}
		a.UserPresence.Observe(failed, time.Now().Unix())
	}
	if required["xray"] {
		rows, e := a.Core.Presence(ctx, users)
		observe("xray", rows, e)
	}
	credentials, err := a.Store.ListProtocolCredentials(ctx)
	if required["hysteria2"] {
		if err == nil && a.Engines != nil {
			rows, e := a.Engines.HysteriaPresence(ctx, users, credentials)
			observe("hysteria2", rows, e)
		} else {
			observe("hysteria2", nil, usage.ErrInvalid)
		}
	}
	if required["wireguard"] {
		if len(a.wireGuardPresence) == 0 && err == nil && a.Engines != nil {
			if counters, _, e := a.Engines.WireGuardStatistics(ctx, users, credentials); e == nil {
				a.cacheWireGuardPresence(counters)
			}
		}
		if len(a.wireGuardPresence) > 0 {
			observe("wireguard", a.wireGuardPresence, nil)
		} else {
			observe("wireguard", nil, usage.ErrInvalid)
		}
	}
}
