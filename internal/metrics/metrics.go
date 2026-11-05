package metrics

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"time"
)

type System struct {
	UptimeSeconds   int64    `json:"uptime_seconds"`
	MemoryTotal     uint64   `json:"memory_total"`
	MemoryAvailable uint64   `json:"memory_available"`
	Load1           float64  `json:"load_1"`
	CPUPercent      *float64 `json:"cpu_percent"`
	DiskFree        *uint64  `json:"disk_free"`
}

func Read() System {
	var s System
	if b, e := os.ReadFile("/proc/uptime"); e == nil {
		fields := strings.Fields(string(b))
		if len(fields) > 0 {
			v, _ := strconv.ParseFloat(fields[0], 64)
			s.UptimeSeconds = int64(v)
		}
	}
	if b, e := os.ReadFile("/proc/loadavg"); e == nil {
		f := strings.Fields(string(b))
		if len(f) > 0 {
			s.Load1, _ = strconv.ParseFloat(f[0], 64)
		}
	}
	if f, e := os.Open("/proc/meminfo"); e == nil {
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			v := strings.Fields(scanner.Text())
			if len(v) < 2 {
				continue
			}
			n, _ := strconv.ParseUint(v[1], 10, 64)
			if v[0] == "MemTotal:" {
				s.MemoryTotal = n * 1024
			}
			if v[0] == "MemAvailable:" {
				s.MemoryAvailable = n * 1024
			}
		}
		f.Close()
	}
	return s
}
func TimeNow() int64 { return time.Now().Unix() }
