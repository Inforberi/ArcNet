package usage

import (
	"sync"
	"testing"
)

func trafficCounter(at, up, down int64) Counter {
	return Counter{UserID: 1, Identity: "11111111-1111-4111-8111-111111111111", Stream: "vless-reality", Epoch: "one", StartedAt: 1, ObservedAt: at, Upload: up, Download: down}
}
func TestUserTrafficRatesGapsAndActivity(t *testing.T) {
	var tracker TrafficTracker
	user := TrafficUser{ID: 1, Identity: trafficCounter(100, 0, 0).Identity, Kinds: []string{"vless-reality"}}
	tracker.Update([]TrafficUser{user}, []Counter{trafficCounter(100, 1000, 2000)}, 100, true)
	current, _, _ := tracker.Detail(1, trafficCounter(1, 0, 0).Identity, 100)
	if current.UploadBPS != nil || current.LastActivityAt != 0 || current.Online != nil {
		t.Fatal("first sample claimed rate/activity/online")
	}
	tracker.Update([]TrafficUser{user}, []Counter{trafficCounter(105, 1050, 2100)}, 105, true)
	current, _, _ = tracker.Detail(1, trafficCounter(1, 0, 0).Identity, 105)
	if current.UploadBPS == nil || *current.UploadBPS != 10 || *current.DownloadBPS != 20 || current.LastActivityAt != 105 || current.Online != nil {
		t.Fatal(current)
	}
	tracker.Update([]TrafficUser{user}, nil, 110, true)
	current, _, _ = tracker.Detail(1, trafficCounter(1, 0, 0).Identity, 110)
	if current.UploadBPS != nil || current.Status != "unavailable" {
		t.Fatal("failed source became zero", current)
	}
	tracker.Update([]TrafficUser{user}, []Counter{trafficCounter(115, 1100, 2200)}, 115, true)
	current, _, _ = tracker.Detail(1, trafficCounter(1, 0, 0).Identity, 115)
	if current.UploadBPS != nil {
		t.Fatal("bridged missing source")
	}
	tracker.Update([]TrafficUser{user}, []Counter{trafficCounter(120, 1100, 2200)}, 120, true)
	current, _, _ = tracker.Detail(1, trafficCounter(1, 0, 0).Identity, 120)
	if current.UploadBPS == nil || *current.UploadBPS != 0 || current.LastActivityAt != 105 {
		t.Fatal("idle is not a fresh activity", current)
	}
	current, _, _ = tracker.Detail(1, trafficCounter(1, 0, 0).Identity, 150)
	if current.UploadBPS != nil || current.Status != "stale" {
		t.Fatal("stale rate visible")
	}
	next := trafficCounter(151, 20, 30)
	next.Epoch = "two"
	next.StartedAt = 150
	tracker.Update([]TrafficUser{user}, []Counter{next}, 151, true)
	current, _, _ = tracker.Detail(1, trafficCounter(1, 0, 0).Identity, 151)
	if current.UploadBPS != nil {
		t.Fatal("restart spike")
	}
}
func TestUserTrafficPartialKindsAndIdentity(t *testing.T) {
	var tracker TrafficTracker
	user := TrafficUser{ID: 1, Identity: trafficCounter(100, 0, 0).Identity, Kinds: []string{"vless-reality", "hysteria2"}}
	for _, at := range []int64{100, 105} {
		tracker.Update([]TrafficUser{user}, []Counter{trafficCounter(at, at, at)}, at, false)
	}
	current, _, complete := tracker.Detail(1, trafficCounter(1, 0, 0).Identity, 105)
	if current.UploadBPS != nil || complete {
		t.Fatal("missing engine became complete rate")
	}
	user.Identity = "22222222-2222-4222-8222-222222222222"
	tracker.Update([]TrafficUser{user}, nil, 110, true)
	_, history, _ := tracker.Detail(1, user.Identity, 110)
	for _, p := range history {
		if p.At < 110 {
			t.Fatal("old identity history leaked")
		}
	}
	tracker.Update(nil, nil, 115, true)
	items, _ := tracker.Snapshot(115, nil)
	if len(items) != 0 {
		t.Fatal("deleted user remains")
	}
}
func TestUserTrafficBoundedAndConcurrent(t *testing.T) {
	var tracker TrafficTracker
	owners := make([]TrafficUser, 200)
	for i := range owners {
		owners[i] = TrafficUser{ID: int64(i + 1), Identity: "11111111-1111-4111-8111-111111111111"}
	}
	for n := int64(1); n < 100; n++ {
		tracker.Update(owners, nil, n*60, true)
	}
	if len(tracker.history) != TrafficHistoryLimit || len(tracker.latest) != len(owners) {
		t.Fatal("unbounded history")
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for n := 0; n < 50; n++ {
			tracker.Update(owners, nil, 10000+int64(n), true)
		}
	}()
	go func() {
		defer wg.Done()
		for n := 0; n < 50; n++ {
			tracker.Snapshot(10010, owners)
			tracker.Detail(1, trafficCounter(1, 0, 0).Identity, 10010)
		}
	}()
	wg.Wait()
}

func TestUserTrafficAggregationAndClock(t *testing.T) {
	var tracker TrafficTracker
	user := TrafficUser{ID: 1, Identity: trafficCounter(1, 0, 0).Identity, Kinds: []string{"vless-reality", "hysteria2"}}
	for _, at := range []int64{100, 105} {
		c := trafficCounter(at, at*10, at*20)
		other := c
		other.Stream = "hysteria2"
		tracker.Update([]TrafficUser{user}, []Counter{c, other}, at, true)
	}
	current, _, _ := tracker.Detail(1, user.Identity, 105)
	if current.UploadBPS == nil || *current.UploadBPS != 20 || *current.DownloadBPS != 40 {
		t.Fatal("did not aggregate protocols", current)
	}
	tracker.Update([]TrafficUser{user}, nil, 105, true)
	current, _, _ = tracker.Detail(1, user.Identity, 105)
	if current.UploadBPS == nil {
		t.Fatal("duplicate timestamp replaced a good sample")
	}
	tracker.Update([]TrafficUser{user}, []Counter{trafficCounter(90, 1100, 2200)}, 90, true)
	current, history, _ := tracker.Detail(1, user.Identity, 90)
	if current.UploadBPS != nil || len(history) != 1 || current.LastActivityAt != 0 {
		t.Fatal("clock rollback retained future data")
	}
}
func BenchmarkUserTrafficUpdate1000(b *testing.B) {
	var tracker TrafficTracker
	owners := make([]TrafficUser, 1000)
	counters := make([]Counter, 1000)
	for i := range owners {
		owners[i] = TrafficUser{ID: int64(i + 1), Identity: trafficCounter(1, 0, 0).Identity, Kinds: []string{"vless-reality"}}
		counters[i] = trafficCounter(100, 1, 2)
		counters[i].UserID = int64(i + 1)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		at := int64(100 + n*5)
		for i := range counters {
			counters[i].ObservedAt = at
			counters[i].Upload = at
			counters[i].Download = at * 2
		}
		tracker.Update(owners, counters, at, true)
	}
}
func BenchmarkUserTrafficSnapshot50(b *testing.B) {
	var tracker TrafficTracker
	owners := make([]TrafficUser, 1000)
	for i := range owners {
		owners[i] = TrafficUser{ID: int64(i + 1), Identity: trafficCounter(1, 0, 0).Identity}
	}
	tracker.Update(owners, nil, 100, true)
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		tracker.Snapshot(100, owners[:50])
	}
}
