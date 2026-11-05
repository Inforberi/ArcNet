package store

import (
	"context"
	"github.com/arcvpn/arcvpn/internal/usage"
	"testing"
	"time"
)

func TestWireGuardFreshnessQuotaAndUnexpectedEpoch(t *testing.T) {
	st, u := usageFixture(t)
	ctx := context.Background()
	now := time.Now().Unix()
	if err := st.UpsertProtocol(ctx, Protocol{Kind: "wireguard", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	x := usage.Counter{UserID: u.ID, Identity: u.UUID, Stream: "vless-reality", Epoch: "xray", StartedAt: 1, ObservedAt: now, Upload: 10, Download: 20}
	if err := st.RecordUsage(ctx, []usage.Counter{x}, now); err != nil {
		t.Fatal(err)
	}
	if fresh, _, err := st.AccountingFresh(ctx); err != nil || fresh {
		t.Fatal("missing WG source was fresh", err)
	}
	wg := x
	wg.Stream = "wireguard"
	wg.Epoch = "wg"
	wg.Upload = 30
	wg.Download = 40
	for i := 0; i < 2; i++ {
		if err := st.RecordEngineUsage(ctx, "wireguard", []usage.Counter{wg}, now, ""); err != nil {
			t.Fatal(err)
		}
	}
	p, err := st.Package(ctx, u.ID)
	if err != nil || p.Upload+p.Download != 100 {
		t.Fatal("double charge", err)
	}
	if err := st.SavePackage(ctx, u.ID, 100, 0, p.Version, false); err != nil {
		t.Fatal(err)
	}
	if allowed, reason, err := st.UserAccess(ctx, u.ID); err != nil || allowed || reason != "quota_exhausted" {
		t.Fatal("quota bypass", allowed, reason, err)
	}
	if err := st.RecordEngineUsage(ctx, "xray", []usage.Counter{wg}, now, ""); err == nil {
		t.Fatal("WG masqueraded as Xray")
	}
	wg.Epoch = "new-wg"
	wg.StartedAt = 2
	wg.Upload = 0
	wg.Download = 0
	if err := st.RecordEngineUsage(ctx, "wireguard", []usage.Counter{wg}, now, ""); err != nil {
		t.Fatal(err)
	}
	p, _ = st.Package(ctx, u.ID)
	if !p.NeedsReview || !p.Incomplete || p.Upload+p.Download != 100 {
		t.Fatal("unexpected restart lost accounting/review gate")
	}
}
