package store

import (
	"context"
	"testing"
	"time"

	"github.com/arcvpn/arcvpn/internal/devices"
	"github.com/arcvpn/arcvpn/internal/usage"
)

func TestHysteriaIndependentAccountingFreshnessAndQuota(t *testing.T) {
	st, u := usageFixture(t)
	ctx := context.Background()
	now := time.Now().Unix()
	if err := st.UpsertProtocol(ctx, Protocol{Kind: "hysteria2", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	x := usage.Counter{UserID: u.ID, Identity: u.UUID, Stream: "vless-reality", Epoch: "xray", StartedAt: 1, ObservedAt: now, Upload: 10, Download: 20}
	if err := st.RecordUsage(ctx, []usage.Counter{x}, now); err != nil {
		t.Fatal(err)
	}
	if fresh, _, err := st.AccountingFresh(ctx); err != nil || fresh {
		t.Fatal("Xray made missing Hysteria fresh", fresh, err)
	}
	h := x
	h.Stream = "hysteria2"
	h.Epoch = "hysteria"
	h.Upload = 30
	h.Download = 40
	if err := st.RecordEngineUsage(ctx, "hysteria", []usage.Counter{h}, now, ""); err != nil {
		t.Fatal(err)
	}
	if fresh, _, err := st.AccountingFresh(ctx); err != nil || !fresh {
		t.Fatal("complete sources not fresh", fresh, err)
	}
	if err := st.RecordEngineUsage(ctx, "hysteria", []usage.Counter{h}, now, ""); err != nil {
		t.Fatal(err)
	}
	p, err := st.Package(ctx, u.ID)
	if err != nil || p.Upload != 40 || p.Download != 60 {
		t.Fatal("replay double charged", p, err)
	}
	if err := st.SavePackage(ctx, u.ID, 100, 0, p.Version, false); err != nil {
		t.Fatal(err)
	}
	if allowed, reason, err := st.UserAccess(ctx, u.ID); err != nil || allowed || reason != "quota_exhausted" {
		t.Fatal("shared quota bypass", allowed, reason, err)
	}
	if err := st.RecordEngineUsage(ctx, "hysteria", []usage.Counter{x}, now, ""); err == nil {
		t.Fatal("source masquerade accepted")
	}
	if err := st.ChangeDevices(ctx, u.ID, 0, func(r *devices.Record) error { return nil }); err != nil {
		t.Fatal("Hysteria device mode rejected", err)
	}
}
