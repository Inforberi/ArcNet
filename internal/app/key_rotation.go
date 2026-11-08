package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"

	"github.com/arcvpn/arcvpn/internal/protocols"
	"github.com/arcvpn/arcvpn/internal/store"
)

type RotationUser struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}
type KeyRotationPlan struct {
	Revision     string         `json:"revision"`
	Confirmation string         `json:"confirmation"`
	Affected     int            `json:"affected"`
	Users        []RotationUser `json:"users"`
}

func (a *App) rotationPlanLocked(ctx context.Context, kind string) (KeyRotationPlan, store.Protocol, error) {
	var p store.Protocol
	var err error
	if kind == "vless-reality" {
		var r store.Reality
		r, err = a.Store.ReadReality(ctx)
		p = RealityProfile(r)
	} else if store.ConnectionKind(kind) == protocols.XHTTP || store.IsAdditionalConnection(kind) || kind == protocols.WireGuard {
		p, err = a.Store.GetProtocol(ctx, kind)
	} else {
		return KeyRotationPlan{}, p, protocols.ErrInvalid
	}
	if err != nil {
		return KeyRotationPlan{}, p, err
	}
	users, err := a.Store.ListUsers(ctx)
	if err != nil {
		return KeyRotationPlan{}, p, err
	}
	affected := []RotationUser{}
	for _, u := range users {
		if u.Allows(kind) {
			affected = append(affected, RotationUser{u.ID, u.Name})
		}
	}
	sort.Slice(affected, func(i, j int) bool { return affected[i].ID < affected[j].ID })
	revision := ConnectionRevision(p)
	blob, err := json.Marshal(struct {
		Kind, Revision string
		Users          []RotationUser
	}{kind, revision, affected})
	if err != nil {
		return KeyRotationPlan{}, p, err
	}
	digest := sha256.Sum256(blob)
	plan := KeyRotationPlan{Revision: revision, Confirmation: hex.EncodeToString(digest[:]), Affected: len(affected), Users: affected}
	if len(plan.Users) > 20 {
		plan.Users = plan.Users[:20]
	}
	return plan, p, nil
}
func (a *App) PreviewKeyRotation(ctx context.Context, kind string) (KeyRotationPlan, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	plan, _, err := a.rotationPlanLocked(ctx, kind)
	return plan, err
}
func (a *App) RotateConnectionKeys(ctx context.Context, kind, revision, confirmation string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	plan, p, err := a.rotationPlanLocked(ctx, kind)
	if err != nil {
		return err
	}
	if revision != plan.Revision || confirmation != plan.Confirmation {
		return ErrConnectionChanged
	}
	a.publication.Add(1)
	defer a.publication.Add(1)
	if err = a.editConnectionLocked(ctx, kind, revision, ConnectionSettings{RotateKeys: true, Port: p.Port, Target: p.Target, SNI: p.SNI, Enabled: p.Enabled}); err != nil {
		return err
	}
	a.Store.Event(ctx, "connection.keys.rotated", kind)
	return nil
}
