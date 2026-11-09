package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/arcvpn/arcvpn/internal/db"
	"github.com/arcvpn/arcvpn/internal/usage"
	"strconv"
	"time"
)

func (s *Store) NodePresenceCapable(ctx context.Context, node string) (bool, error) {
	var capable bool
	err := s.DB.Read(ctx, func(tx *db.Tx) error {
		rows, err := tx.Query("SELECT protocol,checked_at FROM node_presence_capabilities WHERE node_id=?", node)
		if err != nil {
			return err
		}
		if len(rows) == 1 {
			at, _ := strconv.ParseInt(rows[0]["checked_at"], 10, 64)
			capable = rows[0]["protocol"] == "1" && at <= time.Now().Unix()+5 && time.Now().Unix()-at <= 75
		}
		return nil
	})
	return capable, err
}
func (s *Store) NodePresenceSources(ctx context.Context) ([]usage.PresenceSource, error) {
	result := []usage.PresenceSource{}
	err := s.DB.Read(ctx, func(tx *db.Tx) error {
		rows, err := tx.Query("SELECT n.id,c.desired_revision,c.applied_revision,c.desired_digest,c.deployment_status FROM managed_nodes n LEFT JOIN node_control c ON c.node_id=n.id")
		if err != nil {
			return err
		}
		for _, row := range rows {
			result = append(result, usage.PresenceSource{Source: "xray", NodeID: row["id"], ConnectionID: "vless-reality", Binding: row["desired_revision"] + ":" + row["applied_revision"] + ":" + row["desired_digest"] + ":" + row["deployment_status"]})
		}
		return nil
	})
	return result, err
}
func (s *Store) ValidateNodePresence(ctx context.Context, node, secret string, r usage.NodePresenceReport) ([]usage.PresenceObservation, error) {
	now := time.Now()
	if r.Version != 1 || r.NodeID != node || r.Revision < 1 || r.ObservedAt < now.Unix()-usage.FreshFor || r.ObservedAt > now.Unix()+5 || r.StartedAt < 1 || r.StartedAt > now.Add(5*time.Second).UnixNano() || len(r.Epoch) < 1 || len(r.Epoch) > 128 || len(r.Clients) > usage.MaxNodeCounters {
		return nil, usage.ErrInvalid
	}
	result := []usage.PresenceObservation{}
	err := s.DB.Read(ctx, func(tx *db.Tx) error {
		ok, err := s.nodeForSecret(tx, secret, node)
		if err != nil {
			return err
		}
		if !ok {
			return ErrNodeUnauthorized
		}
		if err = requireNodeWithoutCascade(tx, node); err != nil {
			return err
		}
		caps, err := tx.Query("SELECT protocol,checked_at FROM node_presence_capabilities WHERE node_id=?", node)
		if err != nil {
			return err
		}
		if len(caps) != 1 || caps[0]["protocol"] != "1" {
			return ErrUsageUnsupported
		}
		checked, _ := strconv.ParseInt(caps[0]["checked_at"], 10, 64)
		if checked > now.Unix()+5 || now.Unix()-checked > 75 {
			return ErrUsageUnsupported
		}
		rows, err := tx.Query("SELECT c.*,o.kind FROM node_control c JOIN node_operations o ON o.id=c.deployment_id WHERE c.node_id=?", node)
		if err != nil {
			return err
		}
		if len(rows) != 1 {
			return ErrConflict
		}
		state := rows[0]
		if state["kind"] != "apply_xray_usage_v1" || state["deployment_status"] != "applied" || state["desired_revision"] != state["applied_revision"] || state["desired_revision"] != strconv.FormatInt(r.Revision, 10) || state["deployment_id"] != r.OperationID || state["desired_digest"] != r.Digest {
			return ErrConflict
		}
		config, err := s.openNodeConfig(state["desired_ciphertext"], fmt.Sprintf("%s:%d:%s", node, r.Revision, r.Digest))
		if err != nil {
			return err
		}
		digest := sha256.Sum256(config)
		if hex.EncodeToString(digest[:]) != r.Digest {
			return ErrConflict
		}
		if !usage.NodePresenceEnabled(config) {
			return ErrUsageUnsupported
		}
		clients, err := usage.NodeClients(config)
		if err != nil {
			return err
		}
		if len(clients) != len(r.Clients) {
			return usage.ErrInvalid
		}
		users, err := tx.Query("SELECT id,uuid FROM users")
		if err != nil {
			return err
		}
		identities := map[int64]string{}
		credentials := map[int64]map[string]bool{}
		for _, user := range users {
			id, _ := strconv.ParseInt(user["id"], 10, 64)
			identities[id] = user["uuid"]
			credentials[id] = map[string]bool{user["uuid"]: true}
		}
		devices, err := readDevices(tx)
		if err != nil {
			return err
		}
		for _, record := range devices {
			for _, device := range record.Devices {
				if credentials[record.UserID] != nil {
					credentials[record.UserID][device.UUID] = true
				}
			}
		}
		active, seen := map[int64]bool{}, map[string]bool{}
		for _, client := range r.Clients {
			owner, ok := clients[client.Name]
			if !ok || seen[client.Name] || client.Online == nil || !credentials[owner.UserID][owner.Credential] {
				return usage.ErrInvalid
			}
			seen[client.Name] = true
			if *client.Online {
				active[owner.UserID] = true
			}
		}
		binding := state["desired_revision"] + ":" + state["applied_revision"] + ":" + state["desired_digest"] + ":" + state["deployment_status"]
		for id, identity := range identities {
			online := active[id]
			result = append(result, usage.PresenceObservation{UserID: id, Identity: identity, PresenceSource: usage.PresenceSource{Source: "xray", NodeID: node, ConnectionID: "vless-reality", State: "fresh", Binding: binding, Epoch: r.Epoch, StartedAt: r.StartedAt, ObservedAt: r.ObservedAt, Online: &online}})
		}
		return nil
	})
	return result, err
}
