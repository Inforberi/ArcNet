package engines

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/arcvpn/arcvpn/internal/protocols"
	"github.com/arcvpn/arcvpn/internal/store"
	"github.com/arcvpn/arcvpn/internal/usage"
	"io"
)

func ParseHysteriaOnline(data []byte, users []store.User, credentials []store.ProtocolCredential, epoch string, at int64) ([]usage.PresenceObservation, error) {
	if len(data) > 8<<20 {
		return nil, usage.ErrInvalid
	}
	var counts map[string]int64
	d := json.NewDecoder(bytes.NewReader(data))
	if d.Decode(&counts) != nil || counts == nil || d.Decode(new(any)) != io.EOF || len(counts) > 20000 {
		return nil, usage.ErrInvalid
	}
	owners := map[string]int64{}
	for _, id := range protocols.HysteriaIdentities(users, credentials) {
		owners[id.Name] = id.UserID
	}
	active := map[int64]bool{}
	for name, count := range counts {
		id, ok := owners[name]
		if !ok || count < 0 || count > 1000000 {
			return nil, usage.ErrInvalid
		}
		if count > 0 {
			active[id] = true
		}
	}
	result := make([]usage.PresenceObservation, 0, len(users))
	for _, user := range users {
		value := active[user.ID]
		result = append(result, usage.PresenceObservation{UserID: user.ID, Identity: user.UUID, PresenceSource: usage.PresenceSource{Source: "hysteria2", State: "fresh", ObservedAt: at, Epoch: epoch, Online: &value}})
	}
	return result, nil
}
func (m Manager) HysteriaPresence(ctx context.Context, users []store.User, credentials []store.ProtocolCredential) ([]usage.PresenceObservation, error) {
	before, err := hysteriaServiceProcess(ctx)
	if err != nil {
		return nil, err
	}
	data, at, err := m.hysteriaRead(ctx, users, credentials, before, "/online")
	if err != nil {
		return nil, err
	}
	after, err := hysteriaServiceProcess(ctx)
	if err != nil || before != after {
		return nil, usage.ErrInvalid
	}
	return ParseHysteriaOnline(data, users, credentials, before.Epoch, at)
}
