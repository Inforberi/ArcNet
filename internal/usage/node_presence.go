package usage

import "encoding/json"

func NodePresenceEnabled(config []byte) bool {
	var root struct {
		Policy struct {
			Levels map[string]struct {
				Online bool `json:"statsUserOnline"`
			} `json:"levels"`
		} `json:"policy"`
	}
	return json.Unmarshal(config, &root) == nil && root.Policy.Levels["0"].Online
}

type NamedPresence struct {
	Name   string `json:"name"`
	Online *bool  `json:"online"`
}

// Independent telemetry message: no byte counters, addresses, or credentials.
type NodePresenceReport struct {
	Version     int             `json:"version"`
	NodeID      string          `json:"node_id"`
	OperationID string          `json:"operation_id"`
	Revision    int64           `json:"revision"`
	Digest      string          `json:"digest"`
	Epoch       string          `json:"epoch"`
	StartedAt   int64           `json:"started_at"`
	ObservedAt  int64           `json:"observed_at"`
	Clients     []NamedPresence `json:"clients"`
}
