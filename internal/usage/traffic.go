package usage

import "sync"

const TrafficHistoryLimit = 16384
const TrafficHistorySeconds int64 = 2 * 60 * 60

type TrafficUser struct {
	ID       int64
	Identity string
	Kinds    []string
}
type TrafficPoint struct {
	At            int64    `json:"at"`
	UploadBPS     *float64 `json:"upload_bps"`
	DownloadBPS   *float64 `json:"download_bps"`
	WindowSeconds int64    `json:"window_seconds"`
	Segment       int64    `json:"segment"`
}
type UserTraffic struct {
	UserID int64 `json:"user_id"`
	TrafficPoint
	LastActivityAt int64  `json:"last_activity_at"`
	Status         string `json:"status"`
	Online         *bool  `json:"online"`
}
type trafficKey struct {
	id   int64
	kind string
}
type trafficState struct {
	UserTraffic
	identity string
}
type trafficHistory struct {
	id       int64
	identity string
	point    TrafficPoint
}

// TrafficTracker holds bounded, ephemeral observations, separate from authoritative billing.
// Update is called by the existing collector, never by browser requests.
type TrafficTracker struct {
	mu       sync.RWMutex
	previous map[trafficKey]Counter
	latest   map[int64]trafficState
	history  []trafficHistory
	cursor   int
	lastSlot map[int64]int
	complete bool
	lastAt   int64
}

func (t *TrafficTracker) Update(users []TrafficUser, counters []Counter, at int64, complete bool) {
	if len(users) > 10000 || len(counters) > 40000 || at <= 0 {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if at == t.lastAt {
		return
	}
	if at < t.lastAt {
		t.previous = nil
		t.latest = nil
		t.history = nil
		t.cursor = 0
		t.lastSlot = nil
	}
	t.lastAt = at
	observed := make(map[trafficKey]Counter, len(counters))
	for _, c := range counters {
		if c.Source != "" || c.Validate() != nil || c.ObservedAt > at || at-c.ObservedAt > FreshFor {
			continue
		}
		key := trafficKey{c.UserID, c.Stream}
		if _, duplicate := observed[key]; duplicate {
			return
		}
		observed[key] = c
	}
	next := make(map[int64]trafficState, len(users))
	previous := make(map[trafficKey]Counter, len(observed))
	if t.lastSlot == nil {
		t.lastSlot = map[int64]int{}
	}
	for _, u := range users {
		old, known := t.latest[u.ID]
		if known && old.identity != u.Identity {
			old = trafficState{}
			delete(t.lastSlot, u.ID)
		}
		state := trafficState{UserTraffic: UserTraffic{UserID: u.ID, TrafficPoint: TrafficPoint{At: at, Segment: old.Segment}, LastActivityAt: old.LastActivityAt, Status: "collecting"}, identity: u.Identity}
		valid := len(u.Kinds) > 0
		seenAll := len(u.Kinds) > 0
		upRate, downRate := 0.0, 0.0
		for _, kind := range u.Kinds {
			key := trafficKey{u.ID, kind}
			c, ok := observed[key]
			if !ok || c.Identity != u.Identity {
				valid = false
				seenAll = false
				continue
			}
			previous[key] = c
			prev, ok := t.previous[key]
			if !ok || prev.Identity != c.Identity || prev.Epoch != c.Epoch || prev.StartedAt != c.StartedAt || c.ObservedAt <= prev.ObservedAt || c.ObservedAt-prev.ObservedAt > FreshFor {
				valid = false
				continue
			}
			up, down, err := Delta(&prev, c)
			if err != nil {
				valid = false
				continue
			}
			window := c.ObservedAt - prev.ObservedAt
			upRate += float64(up) / float64(window)
			downRate += float64(down) / float64(window)
			state.WindowSeconds = max(state.WindowSeconds, window)
			if up > 0 || down > 0 {
				state.LastActivityAt = max(state.LastActivityAt, c.ObservedAt)
			}
		}
		if valid {
			state.UploadBPS = &upRate
			state.DownloadBPS = &downRate
			state.Status = "fresh"
		} else {
			state.Segment = at
			if !seenAll {
				state.Status = "unavailable"
			}
		}
		next[u.ID] = state
		t.record(u.ID, u.Identity, state.TrafficPoint)
	}
	for id := range t.lastSlot {
		if _, ok := next[id]; !ok {
			delete(t.lastSlot, id)
		}
	}
	t.previous = previous
	t.latest = next
	t.complete = complete
}
func (t *TrafficTracker) record(id int64, identity string, p TrafficPoint) {
	if i, ok := t.lastSlot[id]; ok && i < len(t.history) && t.history[i].id == id && t.history[i].point.At/60 == p.At/60 {
		t.history[i].point = p
		return
	}
	entry := trafficHistory{id, identity, p}
	if len(t.history) < TrafficHistoryLimit {
		t.lastSlot[id] = len(t.history)
		t.history = append(t.history, entry)
		return
	}
	t.history[t.cursor] = entry
	t.lastSlot[id] = t.cursor
	t.cursor = (t.cursor + 1) % TrafficHistoryLimit
}
func publicTraffic(s trafficState, now int64) UserTraffic {
	item := s.UserTraffic
	if now+5 < item.At || now-item.At > FreshFor {
		item.Status = "stale"
		item.UploadBPS = nil
		item.DownloadBPS = nil
	}
	return item
}
func (t *TrafficTracker) Snapshot(now int64, users []TrafficUser) ([]UserTraffic, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	items := make([]UserTraffic, 0, len(users))
	for _, u := range users {
		item := UserTraffic{UserID: u.ID, Status: "unavailable"}
		if s, ok := t.latest[u.ID]; ok && s.identity == u.Identity {
			item = publicTraffic(s, now)
		}
		items = append(items, item)
	}
	return items, t.complete
}

func (t *TrafficTracker) Detail(id int64, identity string, now int64) (UserTraffic, []TrafficPoint, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	current := UserTraffic{UserID: id, Status: "unavailable"}
	if s, ok := t.latest[id]; ok && s.identity == identity {
		current = publicTraffic(s, now)
	}
	if state, ok := t.latest[id]; !ok || state.identity != identity {
		return current, []TrafficPoint{}, t.complete
	}
	history := make([]TrafficPoint, 0, 121)
	// The ring is chronological; filtering is bounded independently of fleet size.
	start := 0
	if len(t.history) == TrafficHistoryLimit {
		start = t.cursor
	}
	for n := 0; n < len(t.history); n++ {
		entry := t.history[(start+n)%len(t.history)]
		if entry.id == id && entry.identity == identity && entry.point.At >= now-TrafficHistorySeconds && entry.point.At <= now {
			history = append(history, entry.point)
		}
	}
	return current, history, t.complete
}
