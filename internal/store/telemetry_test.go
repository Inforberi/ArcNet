package store

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/arcvpn/arcvpn/internal/db"
	"github.com/arcvpn/arcvpn/internal/metrics"
)

func TestTelemetryIsNodeBoundThrottledAndDoesNotAssertVPN(t *testing.T) {
	st, done := createControlTestStore(t)
	defer done()
	ctx := context.Background()
	node, secret := createControlTestNode(t, st)
	sample := metrics.Snapshot{SampledAt: time.Now().Unix(), Epoch: "fixture", CPUs: 2}
	if err := st.RecordTelemetry(ctx, "local", secret, sample); !errors.Is(err, ErrNodeUnauthorized) {
		t.Fatal("remote node claimed local metrics")
	}
	if err := st.RecordTelemetry(ctx, node, "wrong-secret", sample); !errors.Is(err, ErrNodeUnauthorized) {
		t.Fatal("foreign secret accepted")
	}
	if err := st.RecordTelemetry(ctx, node, secret, sample); err != nil {
		t.Fatal(err)
	}
	if err := st.RecordTelemetry(ctx, node, secret, sample); !errors.Is(err, ErrTelemetryRateLimit) {
		t.Fatal("agent not rate limited")
	}
	got, err := st.LatestTelemetry(ctx, node)
	if err != nil || got == nil || got.Snapshot.CPUPercent != nil {
		t.Fatalf("unknown fabricated: %#v %v", got, err)
	}
	control, err := st.NodeControl(ctx, node)
	if err != nil || control.Probe != "unknown" || control.Published {
		t.Fatal("metrics became a VPN proof")
	}
	if err := st.RemoveNode(ctx, node); err != nil {
		t.Fatal(err)
	}
	if got, _ := st.LatestTelemetry(ctx, node); got != nil {
		t.Fatal("revoked node telemetry retained")
	}
}
func TestTelemetryHistoryRetentionAndRestoreDoesNotRestoreObservations(t *testing.T) {
	st, done := createControlTestStore(t)
	defer done()
	ctx := context.Background()
	now := time.Now().Unix()
	err := st.DB.WithTx(ctx, func(tx *db.Tx) error {
		for i := 0; i < 130; i++ {
			if err := tx.Exec("INSERT INTO telemetry_samples(node_id,bucket,received_at,body) VALUES('old',?,?,'{}')", strconv.FormatInt(now/60-int64(i), 10), strconv.FormatInt(now-int64(i*60), 10)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = st.RecordLocalTelemetry(ctx, metrics.Snapshot{SampledAt: now, CPUs: 1}); err != nil {
		t.Fatal(err)
	}
	history, err := st.TelemetryHistory(ctx, "old")
	if err != nil || len(history) != 120 {
		t.Fatalf("history bound: %d %v", len(history), err)
	}
	empty := []NodeIdentity{}
	if err = st.ReplaceFullStateWithManagement(ctx, nil, nil, nil, nil, &empty, nil); err != nil {
		t.Fatal(err)
	}
	if history, _ = st.TelemetryHistory(ctx, "old"); len(history) != 0 {
		t.Fatal("identity restore preserved stale remote observations")
	}
	if history, _ = st.TelemetryHistory(ctx, "local"); len(history) != 1 {
		t.Fatal("restore erased controller history")
	}
}

func TestTelemetryRecomputesRatesAndResetsOnInterfaceChange(t *testing.T) {
	state, done := createControlTestStore(t)
	defer done()
	ctx := context.Background()
	node, secret := createControlTestNode(t, state)
	now := time.Now().Unix()
	rx, tx := uint64(1000), uint64(2000)
	claimed := 999999.0
	snapshot := metrics.Snapshot{SampledAt: now - 20, Epoch: "boot-a", CPUs: 1, NetworkInterfaces: []string{"eth0"}, RXBytes: &rx, TXBytes: &tx, RXPerSecond: &claimed, TXPerSecond: &claimed}
	if err := state.RecordTelemetry(ctx, node, secret, snapshot); err != nil {
		t.Fatal(err)
	}
	first, _ := state.LatestTelemetry(ctx, node)
	if first.Snapshot.RXPerSecond != nil {
		t.Fatal("first report invented a rate")
	}
	expireThrottle := func() {
		t.Helper()
		if err := state.DB.WithTx(ctx, func(tx *db.Tx) error {
			return tx.Exec("UPDATE telemetry_samples SET received_at=? WHERE node_id=?", strconv.FormatInt(now-20, 10), node)
		}); err != nil {
			t.Fatal(err)
		}
	}
	expireThrottle()
	snapshot.SampledAt = now
	rx = 1200
	tx = 2600
	if err := state.RecordTelemetry(ctx, node, secret, snapshot); err != nil {
		t.Fatal(err)
	}
	second, _ := state.LatestTelemetry(ctx, node)
	if second.Snapshot.RXPerSecond == nil || *second.Snapshot.RXPerSecond != 10 || *second.Snapshot.TXPerSecond != 30 {
		t.Fatal("reported rates trusted instead of counter deltas")
	}
	expireThrottle()
	snapshot.SampledAt = now + 20
	snapshot.NetworkInterfaces = []string{"eth1"}
	rx = 999999
	tx = 999999
	if err := state.RecordTelemetry(ctx, node, secret, snapshot); err != nil {
		t.Fatal(err)
	}
	changed, _ := state.LatestTelemetry(ctx, node)
	if changed.Snapshot.RXPerSecond != nil {
		t.Fatal("interface replacement produced a false network spike")
	}
}

func TestTelemetryAcceptsFiveSecondCadenceWithoutUnboundedWrites(t *testing.T) {
	state, done := createControlTestStore(t)
	defer done()
	ctx := context.Background()
	node, _ := createControlTestNode(t, state)
	now := time.Now().Unix()
	write := func(at int64) error {
		return state.DB.WithTx(ctx, func(tx *db.Tx) error {
			return writeTelemetry(tx, node, metrics.Snapshot{SampledAt: at, CPUs: 1}, at, true)
		})
	}
	if err := write(now - 10); err != nil {
		t.Fatal(err)
	}
	if err := write(now - 6); !errors.Is(err, ErrTelemetryRateLimit) {
		t.Fatal("sub-five-second write accepted", err)
	}
	if err := write(now - 5); err != nil {
		t.Fatal("five-second report rejected", err)
	}
}
