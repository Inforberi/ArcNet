package metrics

import (
	"errors"
	"math"
	"runtime"
	"testing"
	"time"
)

func samplerFixture() (*Sampler, map[string]string, *time.Time) {
	now := time.Unix(1800000000, 0)
	files := map[string]string{
		"/proc/sys/kernel/random/boot_id": "boot-a\n", "/proc/stat": "cpu 100 0 0 900 0 0 0 0 0 0\n",
		"/proc/uptime": "100.3 0\n", "/proc/loadavg": "0.5 0.2 0.1 1/10 2",
		"/proc/meminfo":       "MemTotal: 1000 kB\nMemAvailable: 500 kB\nSwapTotal: 0 kB\nSwapFree: 0 kB\n",
		"/proc/net/dev":       "eth0: 1000 0 0 0 0 0 0 0 2000 0 0 0 0 0 0 0\nlo: 999999 0 0 0 0 0 0 0 999999 0 0 0 0 0 0 0",
		"/proc/net/route":     "Iface Destination Gateway Flags\neth0 00000000 0100000A 0003\n",
		"/proc/net/sockstat":  "TCP: inuse 3 orphan 0\nUDP: inuse 4 mem 0",
		"/proc/net/sockstat6": "TCP6: inuse 2\nUDP6: inuse 2\n",
	}
	s := &Sampler{cpus: 2, now: func() time.Time { return now }, read: func(path string) ([]byte, error) {
		v, ok := files[path]
		if !ok {
			return nil, errors.New("unavailable")
		}
		return []byte(v), nil
	}, disk: func() (uint64, uint64, error) { return 1000, 400, nil }}
	return s, files, &now
}
func TestSamplerUsesDeltasCacheAndEpochWithoutInventingZeros(t *testing.T) {
	sampler, files, now := samplerFixture()
	first := sampler.Sample()
	if first.CPUPercent != nil || first.RXPerSecond != nil || first.MemoryTotal == nil || *first.MemoryTotal != 1024000 || first.TCPSockets == nil || *first.TCPSockets != 5 {
		t.Fatalf("first sample: %#v", first)
	}
	files["/proc/stat"] = "cpu 140 0 0 960 0 0 0 0\n"
	files["/proc/net/dev"] = "eth0: 1100 0 0 0 0 0 0 0 2300 0 0 0 0 0 0 0"
	*now = now.Add(500 * time.Millisecond)
	if cached := sampler.Sample(); cached.SampledAt != first.SampledAt || *cached.RXBytes != 1000 {
		t.Fatal("sample was not cached")
	}
	*now = now.Add(9500 * time.Millisecond)
	second := sampler.Sample()
	if second.CPUPercent == nil || *second.CPUPercent != 40 || second.RXPerSecond == nil || *second.RXPerSecond != 10 || *second.TXPerSecond != 30 {
		t.Fatalf("delta: %#v", second)
	}
	files["/proc/sys/kernel/random/boot_id"] = "boot-b"
	files["/proc/stat"] = "cpu 1 0 0 10 0 0 0 0"
	*now = now.Add(10 * time.Second)
	if reboot := sampler.Sample(); reboot.CPUPercent != nil || reboot.RXPerSecond != nil {
		t.Fatal("counter restart produced a false spike")
	}
}
func TestSamplerUnknownAndInvalidValues(t *testing.T) {
	sampler, files, _ := samplerFixture()
	for name := range files {
		delete(files, name)
	}
	sampler.disk = func() (uint64, uint64, error) { return 0, 0, errors.New("no disk") }
	unknown := sampler.Sample()
	if unknown.CPUPercent != nil || unknown.MemoryTotal != nil || unknown.DiskTotal != nil || unknown.TCPSockets != nil || unknown.Load1 != nil {
		t.Fatal("unknown counters became zero")
	}
	if err := unknown.Validate(); err != nil {
		t.Fatal(err)
	}
	bad := unknown
	bad.CPUPercent = decimal(math.NaN())
	if err := bad.Validate(); err == nil {
		t.Fatal("NaN accepted")
	}
	bad.CPUPercent = decimal(101)
	if err := bad.Validate(); err == nil {
		t.Fatal("invalid CPU percentage")
	}
	bad = unknown
	bad.MemoryTotal = integer(5)
	bad.MemoryAvailable = integer(6)
	if err := bad.Validate(); err == nil {
		t.Fatal("impossible memory accepted")
	}
}
func TestNetworkUsesDefaultRouteAndDropsTunnelDoubleCounting(t *testing.T) {
	sampler, files, _ := samplerFixture()
	files["/proc/net/dev"] += "\nwg0: 1000000 0 0 0 0 0 0 0 1000000 0 0 0 0 0 0 0\nbr0: 1000000 0 0 0 0 0 0 0 1000000 0 0 0 0 0 0 0"
	sample := sampler.Sample()
	if sample.RXBytes == nil || *sample.RXBytes != 1000 || *sample.TXBytes != 2000 {
		t.Fatal("virtual/non-default interfaces counted twice")
	}
}
func BenchmarkSamplerCached(b *testing.B) {
	sampler, _, _ := samplerFixture()
	sampler.Sample()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sampler.Sample()
	}
}
func BenchmarkSamplerCollected(b *testing.B) {
	sampler, _, now := samplerFixture()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		*now = now.Add(CacheInterval)
		sampler.Sample()
	}
}

func BenchmarkSamplerLinuxProc(b *testing.B) {
	if runtime.GOOS != "linux" {
		b.Skip("Linux procfs benchmark")
	}
	sampler := NewSampler(b.TempDir())
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sampler.lastAt = time.Time{}
		sampler.Sample()
	}
}
