package usage

import (
	"sort"
	"sync"
)

type PresenceSource struct {
	Source          string `json:"source"`
	NodeID          string `json:"node_id,omitempty"`
	ConnectionID    string `json:"connection_id,omitempty"`
	State           string `json:"state"`
	ObservedAt      int64  `json:"observed_at"`
	LastHandshakeAt int64  `json:"last_handshake_at,omitempty"`
	Epoch           string `json:"-"`
	Binding         string `json:"-"`
	StartedAt       int64  `json:"-"`
	Online          *bool  `json:"-"`
}

func (s PresenceSource) Key() string { return s.NodeID + "/" + s.Source }

type PresenceUser struct {
	ID             int64
	Identity       string
	Sources        []PresenceSource
	LastActivityAt int64
}
type PresenceObservation struct {
	UserID   int64
	Identity string
	PresenceSource
}
type Presence struct {
	State            string           `json:"state"`
	ObservedAt       int64            `json:"observed_at"`
	LastActivityAt   int64            `json:"last_activity_at,omitempty"`
	CompleteCoverage bool             `json:"complete_coverage"`
	Sources          []PresenceSource `json:"sources"`
}
type PresenceSummary struct {
	ExpiresAt int64 `json:"expires_at"`
	Online    int   `json:"online"`
	Known     int   `json:"known"`
	Total     int   `json:"total"`
}
type presenceOwner struct {
	identity string
	sources  map[string]PresenceSource
	activity int64
}

// Ephemeral presence is independent of billing and never stores addresses or secrets.
type PresenceTracker struct {
	mu    sync.RWMutex
	users map[int64]presenceOwner
}

func (t *PresenceTracker) Forget(id int64) { t.mu.Lock(); defer t.mu.Unlock(); delete(t.users, id) }

func (t *PresenceTracker) Roster(users []PresenceUser) {
	if len(users) > 10000 {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	next := make(map[int64]presenceOwner, len(users))
	sources := 0
	for _, user := range users {
		owner := presenceOwner{identity: user.Identity, sources: map[string]PresenceSource{}, activity: user.LastActivityAt}
		old := t.users[user.ID]
		for _, source := range user.Sources {
			sources++
			if sources > 40000 {
				return
			}
			prior, ok := old.sources[source.Key()]
			if ok && old.identity == user.Identity && prior.ConnectionID == source.ConnectionID && prior.Binding == source.Binding {
				source = prior
			}
			owner.sources[source.Key()] = source
		}
		next[user.ID] = owner
	}
	t.users = next
}
func (t *PresenceTracker) Observe(rows []PresenceObservation, now int64) {
	if len(rows) > 40000 {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, row := range rows {
		owner, ok := t.users[row.UserID]
		if !ok || owner.identity != row.Identity {
			continue
		}
		old, ok := owner.sources[row.Key()]
		if !ok {
			continue
		}
		if row.ObservedAt == 0 {
			old.Online = nil
			old.State = "unknown"
			if old.ObservedAt > 0 {
				old.State = "stale"
			}
			owner.sources[row.Key()] = old
			continue
		}
		if row.ObservedAt > now+5 || row.ObservedAt < now-FreshFor || row.ObservedAt < old.ObservedAt {
			continue
		}
		if old.StartedAt > 0 && (row.StartedAt < old.StartedAt || (row.Epoch != old.Epoch && row.StartedAt <= old.StartedAt)) {
			continue
		}
		row.ConnectionID = old.ConnectionID
		owner.sources[row.Key()] = row.PresenceSource
	}
}
func presenceSnapshot(owner presenceOwner, now int64) Presence {
	p := Presence{State: "unknown", Sources: []PresenceSource{}, LastActivityAt: owner.activity, CompleteCoverage: len(owner.sources) > 0}
	online, recent, stale := false, owner.activity > 0 && owner.activity <= now+5 && now-owner.activity <= 60, false
	for _, source := range owner.sources {
		fresh := source.ObservedAt > 0 && source.ObservedAt <= now+5 && now-source.ObservedAt <= FreshFor && source.State != "stale" && source.State != "unknown"
		if !fresh {
			source.Online = nil
			p.CompleteCoverage = false
			if source.ObservedAt > 0 {
				source.State = "stale"
				stale = true
			} else {
				source.State = "unknown"
			}
		} else if source.Online != nil {
			if *source.Online {
				source.State = "online"
				online = true
			} else {
				source.State = "offline"
			}
		} else {
			p.CompleteCoverage = false
			source.State = "unknown"
			if source.LastHandshakeAt > 0 && source.LastHandshakeAt <= now+5 && now-source.LastHandshakeAt <= 180 {
				source.State = "recent"
				recent = true
			}
		}
		if source.ObservedAt > p.ObservedAt {
			p.ObservedAt = source.ObservedAt
		}
		p.Sources = append(p.Sources, source)
	}
	switch {
	case online:
		p.State = "online"
	case recent:
		p.State = "recent"
	case stale:
		p.State = "stale"
	case p.CompleteCoverage:
		p.State = "offline"
	}
	sort.Slice(p.Sources, func(i, j int) bool { return p.Sources[i].Key() < p.Sources[j].Key() })
	return p
}
func (t *PresenceTracker) Snapshot(users []TrafficUser, now int64) (map[int64]Presence, PresenceSummary) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	items := make(map[int64]Presence, len(users))
	summary := PresenceSummary{Total: len(t.users)}
	for _, owner := range t.users {
		p := presenceSnapshot(owner, now)
		if p.State == "online" {
			summary.Online++
		}
		if p.State == "online" || p.State == "offline" {
			summary.Known++
			until := int64(0)
			for _, source := range p.Sources {
				if source.State == "online" && p.State == "online" {
					if source.ObservedAt+FreshFor > until {
						until = source.ObservedAt + FreshFor
					}
				} else if p.State == "offline" && (until == 0 || source.ObservedAt+FreshFor < until) {
					until = source.ObservedAt + FreshFor
				}
			}
			if summary.ExpiresAt == 0 || until < summary.ExpiresAt {
				summary.ExpiresAt = until
			}
		}
	}
	for _, user := range users {
		owner, ok := t.users[user.ID]
		if ok && owner.identity == user.Identity {
			items[user.ID] = presenceSnapshot(owner, now)
		} else {
			items[user.ID] = Presence{State: "unknown", Sources: []PresenceSource{}}
		}
	}
	return items, summary
}
