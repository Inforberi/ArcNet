package engines

import (
	"github.com/arcvpn/arcvpn/internal/usage"
	"strconv"
	"strings"
)

func ParseWireGuardHandshakes(data []byte, now int64) (map[string]int64, error) {
	if len(data) > 8<<20 {
		return nil, usage.ErrInvalid
	}
	result := map[string]int64{}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 || !quickKey(fields[0]) || len(result) >= 20000 {
			return nil, usage.ErrInvalid
		}
		if _, ok := result[fields[0]]; ok {
			return nil, usage.ErrInvalid
		}
		stamp, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil || stamp < 0 || stamp > now+5 {
			return nil, usage.ErrInvalid
		}
		result[fields[0]] = stamp
	}
	return result, nil
}
