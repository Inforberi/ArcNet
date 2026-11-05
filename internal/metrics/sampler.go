package metrics

import (
	"bufio"
	"errors"
	"io"
	"math"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const CacheInterval = 5 * time.Second
const CollectionInterval = 30 * time.Second
const AgentInterval = 60 * time.Second
const FreshForSeconds = 180

// Snapshot uses null for unavailable or not-yet-measurable values. These are
// host/namespace counters, not VPN-user accounting or a connectivity proof.
type Snapshot struct {
	NetworkInterfaces []string `json:"network_interfaces"`
	SampledAt         int64    `json:"sampled_at"`
	Epoch             string   `json:"epoch"`
	CPUs              int      `json:"cpus"`
	CPUPercent        *float64 `json:"cpu_percent"`
	MemoryTotal       *uint64  `json:"memory_total"`
	MemoryAvailable   *uint64  `json:"memory_available"`
	SwapTotal         *uint64  `json:"swap_total"`
	SwapFree          *uint64  `json:"swap_free"`
	DiskTotal         *uint64  `json:"disk_total"`
	DiskFree          *uint64  `json:"disk_free"`
	Load1             *float64 `json:"load_1"`
	UptimeSeconds     *int64   `json:"uptime_seconds"`
	RXBytes           *uint64  `json:"rx_bytes"`
	TXBytes           *uint64  `json:"tx_bytes"`
	RXPerSecond       *float64 `json:"rx_per_second"`
	TXPerSecond       *float64 `json:"tx_per_second"`
	TCPSockets        *uint64  `json:"tcp_sockets"`
	UDPSockets        *uint64  `json:"udp_sockets"`
}
type cpuCounters struct{ total, idle uint64 }
type Sampler struct {
	mu              sync.Mutex
	read            func(string) ([]byte, error)
	disk            func() (uint64, uint64, error)
	now             func() time.Time
	cpus            int
	last            Snapshot
	lastAt          time.Time
	previousCPU     *cpuCounters
	previousNetwork string
}

func NewSampler(path string) *Sampler {
	return &Sampler{read: readProc, now: time.Now, cpus: runtime.NumCPU(), disk: func() (uint64, uint64, error) {
		var stat syscall.Statfs_t
		if err := syscall.Statfs(path, &stat); err != nil {
			return 0, 0, err
		}
		if stat.Bsize <= 0 || uint64(stat.Blocks) > math.MaxUint64/uint64(stat.Bsize) || uint64(stat.Bavail) > uint64(stat.Blocks) {
			return 0, 0, errors.New("invalid filesystem counters")
		}
		return uint64(stat.Blocks) * uint64(stat.Bsize), uint64(stat.Bavail) * uint64(stat.Bsize), nil
	}}
}
func readProc(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 65537))
	if err != nil || len(b) > 65536 {
		return nil, errors.New("unavailable proc counters")
	}
	return b, nil
}
func integer(v uint64) *uint64   { return &v }
func decimal(v float64) *float64 { return &v }
func elapsed(v int64) *int64     { return &v }
func nonnegative(raw string) (float64, bool) {
	v, e := strconv.ParseFloat(raw, 64)
	return v, e == nil && !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0
}

func (s *Sampler) Sample() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	if !s.lastAt.IsZero() && now.Sub(s.lastAt) >= 0 && now.Sub(s.lastAt) < CacheInterval {
		return s.last
	}
	result := Snapshot{SampledAt: now.Unix(), CPUs: s.cpus}
	if b, e := s.read("/proc/sys/kernel/random/boot_id"); e == nil && len(b) <= 128 {
		result.Epoch = strings.TrimSpace(string(b))
	}
	if b, e := s.read("/proc/uptime"); e == nil {
		f := strings.Fields(string(b))
		if len(f) > 0 {
			if v, ok := nonnegative(f[0]); ok && v < math.MaxInt64 {
				result.UptimeSeconds = elapsed(int64(v))
			}
		}
	}
	if b, e := s.read("/proc/loadavg"); e == nil {
		f := strings.Fields(string(b))
		if len(f) > 0 {
			if v, ok := nonnegative(f[0]); ok {
				result.Load1 = decimal(v)
			}
		}
	}
	if b, e := s.read("/proc/meminfo"); e == nil {
		for _, line := range strings.Split(string(b), "\n") {
			f := strings.Fields(line)
			if len(f) != 3 || f[2] != "kB" {
				continue
			}
			v, err := strconv.ParseUint(f[1], 10, 64)
			if err != nil || v > math.MaxUint64/1024 {
				continue
			}
			v *= 1024
			switch f[0] {
			case "MemTotal:":
				result.MemoryTotal = integer(v)
			case "MemAvailable:":
				result.MemoryAvailable = integer(v)
			case "SwapTotal:":
				result.SwapTotal = integer(v)
			case "SwapFree:":
				result.SwapFree = integer(v)
			}
		}
	}
	if total, free, err := s.disk(); err == nil {
		result.DiskTotal = integer(total)
		result.DiskFree = integer(free)
	}
	cpu := parseCPU(s.read)
	dt := now.Sub(s.lastAt).Seconds()
	sameEpoch := result.Epoch != "" && result.Epoch == s.last.Epoch
	if cpu != nil && s.previousCPU != nil && sameEpoch && dt > 0 && dt <= 300 && cpu.total > s.previousCPU.total && cpu.idle >= s.previousCPU.idle {
		total, idle := cpu.total-s.previousCPU.total, cpu.idle-s.previousCPU.idle
		if idle <= total {
			result.CPUPercent = decimal(100 * float64(total-idle) / float64(total))
		}
	}
	rx, tx, signature, ok := parseNetwork(s.read)
	if ok {
		result.NetworkInterfaces = strings.Split(signature, ",")
		result.RXBytes = integer(rx)
		result.TXBytes = integer(tx)
		if sameEpoch && signature == s.previousNetwork && s.last.RXBytes != nil && s.last.TXBytes != nil && rx >= *s.last.RXBytes && tx >= *s.last.TXBytes && dt > 0 && dt <= 300 {
			result.RXPerSecond = decimal(float64(rx-*s.last.RXBytes) / dt)
			result.TXPerSecond = decimal(float64(tx-*s.last.TXBytes) / dt)
		}
	}
	result.TCPSockets, result.UDPSockets = parseSockets(s.read)
	s.last = result
	s.lastAt = now
	s.previousCPU = cpu
	s.previousNetwork = signature
	return result
}
func parseCPU(read func(string) ([]byte, error)) *cpuCounters {
	b, e := read("/proc/stat")
	if e != nil {
		return nil
	}
	line, _, _ := strings.Cut(string(b), "\n")
	f := strings.Fields(line)
	if len(f) < 5 || f[0] != "cpu" {
		return nil
	}
	var total, idle uint64
	for i := 1; i < len(f) && i <= 8; i++ {
		v, e := strconv.ParseUint(f[i], 10, 64)
		if e != nil || v > math.MaxUint64-total {
			return nil
		}
		total += v
		if i == 4 || i == 5 {
			idle += v
		}
	}
	return &cpuCounters{total: total, idle: idle}
}
func parseNetwork(read func(string) ([]byte, error)) (uint64, uint64, string, bool) {
	b, e := read("/proc/net/dev")
	if e != nil {
		return 0, 0, "", false
	}
	var rx, tx uint64
	names := []string{}
	routes := defaultInterfaces(read)
	if len(routes) == 0 {
		return 0, 0, "", false
	}
	for _, line := range strings.Split(string(b), "\n") {
		name, rest, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		if !routes[name] {
			continue
		}
		if name == "lo" || strings.HasPrefix(name, "veth") || strings.HasPrefix(name, "docker") || strings.HasPrefix(name, "br-") || strings.HasPrefix(name, "tun") || strings.HasPrefix(name, "tap") || strings.HasPrefix(name, "wg") || strings.HasPrefix(name, "awg") || strings.HasPrefix(name, "arcwg") || strings.HasPrefix(name, "arcawg") {
			continue
		}
		f := strings.Fields(rest)
		if len(f) < 16 {
			return 0, 0, "", false
		}
		r, e := strconv.ParseUint(f[0], 10, 64)
		if e != nil {
			return 0, 0, "", false
		}
		t, e := strconv.ParseUint(f[8], 10, 64)
		if e != nil || r > math.MaxUint64-rx || t > math.MaxUint64-tx {
			return 0, 0, "", false
		}
		rx += r
		tx += t
		names = append(names, name)
	}
	if len(names) == 0 {
		return 0, 0, "", false
	}
	sort.Strings(names)
	return rx, tx, strings.Join(names, ","), true
}
func defaultInterfaces(read func(string) ([]byte, error)) map[string]bool {
	result := map[string]bool{}
	if b, err := read("/proc/net/route"); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			f := strings.Fields(line)
			if len(f) >= 4 && f[1] == "00000000" {
				flags, e := strconv.ParseUint(f[3], 16, 32)
				if e == nil && flags&1 != 0 {
					result[f[0]] = true
				}
			}
		}
	}
	if b, err := read("/proc/net/ipv6_route"); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			f := strings.Fields(line)
			if len(f) == 10 && f[0] == strings.Repeat("0", 32) && f[1] == "00" && f[9] != "lo" {
				flags, e := strconv.ParseUint(f[8], 16, 32)
				if e == nil && flags&1 != 0 && flags&0x200 == 0 {
					result[f[9]] = true
				}
			}
		}
	}
	return result
}
func parseSockets(read func(string) ([]byte, error)) (*uint64, *uint64) {
	var tcp, udp uint64
	gotTCP, gotUDP := false, false
	for _, path := range []string{"/proc/net/sockstat", "/proc/net/sockstat6"} {
		b, e := read(path)
		if e != nil {
			continue
		}
		scanner := bufio.NewScanner(strings.NewReader(string(b)))
		for scanner.Scan() {
			f := strings.Fields(scanner.Text())
			if len(f) < 3 || f[1] != "inuse" {
				continue
			}
			n, e := strconv.ParseUint(f[2], 10, 64)
			if e != nil {
				continue
			}
			switch f[0] {
			case "TCP:", "TCP6:":
				if n <= math.MaxUint64-tcp {
					tcp += n
					gotTCP = true
				}
			case "UDP:", "UDP6:":
				if n <= math.MaxUint64-udp {
					udp += n
					gotUDP = true
				}
			}
		}
	}
	var t, u *uint64
	if gotTCP {
		t = integer(tcp)
	}
	if gotUDP {
		u = integer(udp)
	}
	return t, u
}

func (s Snapshot) Validate() error {
	if len(s.NetworkInterfaces) > 32 {
		return errors.New("invalid network interface count")
	}
	for _, name := range s.NetworkInterfaces {
		if len(name) < 1 || len(name) > 64 || strings.ContainsAny(name, "\x00\r\n/\\") {
			return errors.New("invalid network interface")
		}
	}
	if s.SampledAt <= 0 || s.CPUs < 1 || s.CPUs > 1048576 || len(s.Epoch) > 128 || strings.ContainsAny(s.Epoch, "\x00\r\n") {
		return errors.New("invalid telemetry metadata")
	}
	for _, value := range []*float64{s.CPUPercent, s.Load1, s.RXPerSecond, s.TXPerSecond} {
		if value != nil && (math.IsNaN(*value) || math.IsInf(*value, 0) || *value < 0 || *value > 1e15) {
			return errors.New("invalid telemetry number")
		}
	}
	if s.CPUPercent != nil && *s.CPUPercent > 100 {
		return errors.New("invalid CPU percent")
	}
	for _, pair := range [][2]*uint64{{s.MemoryTotal, s.MemoryAvailable}, {s.SwapTotal, s.SwapFree}, {s.DiskTotal, s.DiskFree}} {
		if pair[0] != nil && pair[1] != nil && *pair[1] > *pair[0] {
			return errors.New("invalid capacity")
		}
	}
	if s.UptimeSeconds != nil && *s.UptimeSeconds < 0 {
		return errors.New("invalid uptime")
	}
	return nil
}
