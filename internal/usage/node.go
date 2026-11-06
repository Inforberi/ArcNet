package usage

import (
	"encoding/json"
	"regexp"
	"strconv"
)

const MaxNodeCounters = 512

var nodeEmail = regexp.MustCompile(`^user-([1-9][0-9]{0,15})(?:-device-[0-9a-f-]{36})?@local$`)

type NamedCounter struct {
	Name     string `json:"name"`
	Upload   int64  `json:"upload"`
	Download int64  `json:"download"`
}

type NodeReport struct {
	Version     int            `json:"version"`
	NodeID      string         `json:"node_id"`
	OperationID string         `json:"operation_id"`
	Revision    int64          `json:"revision"`
	Digest      string         `json:"digest"`
	Epoch       string         `json:"epoch"`
	StartedAt   int64          `json:"started_at"`
	ObservedAt  int64          `json:"observed_at"`
	Counters    []NamedCounter `json:"counters"`
}

type NodeClient struct {
	UserID     int64
	Credential string
}

// The server and helper derive the roster from the exact applied configuration,
// never from report-supplied user IDs or credential ownership.
func NodeClients(config []byte) (map[string]NodeClient, error) {
	var root struct {
		Inbounds []struct {
			Tag      string `json:"tag"`
			Settings struct {
				Clients []struct {
					ID    string `json:"id"`
					Email string `json:"email"`
				} `json:"clients"`
			} `json:"settings"`
		} `json:"inbounds"`
	}
	if json.Unmarshal(config, &root) != nil || len(root.Inbounds) != 1 || root.Inbounds[0].Tag != "vless-reality" {
		return nil, ErrInvalid
	}
	clients := root.Inbounds[0].Settings.Clients
	if len(clients) > MaxNodeCounters {
		return nil, ErrInvalid
	}
	out := make(map[string]NodeClient, len(clients))
	for _, c := range clients {
		m := nodeEmail.FindStringSubmatch(c.Email)
		if len(m) != 2 || !nodeSource.MatchString(c.ID) {
			return nil, ErrInvalid
		}
		id, e := strconv.ParseInt(m[1], 10, 64)
		if e != nil || id < 1 {
			return nil, ErrInvalid
		}
		if _, exists := out[c.Email]; exists {
			return nil, ErrInvalid
		}
		out[c.Email] = NodeClient{UserID: id, Credential: c.ID}
	}
	return out, nil
}
