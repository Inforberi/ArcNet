package store

import (
	"context"
	"testing"

	"github.com/arcvpn/arcvpn/internal/db"
)

func TestMonitoringDefaultsPersistenceAndAuditRollback(t *testing.T) {
	s, done := createControlTestStore(t)
	defer done()
	ctx := context.Background()
	if n, err := s.MonitoringSeconds(ctx); err != nil || n != 5 {
		t.Fatal(n, err)
	}
	for _, n := range []int{5, 10, 15, 30, 60} {
		if err := s.SetMonitoringSeconds(ctx, n); err != nil {
			t.Fatal(err)
		}
		if got, _ := s.MonitoringSeconds(ctx); got != n {
			t.Fatal("not persisted")
		}
	}
	for _, n := range []int{-1, 0, 1, 6, 300} {
		if s.SetMonitoringSeconds(ctx, n) == nil {
			t.Fatal("bad interval accepted", n)
		}
	}
	if err := s.DB.WithTx(ctx, func(tx *db.Tx) error {
		return tx.Exec("CREATE TRIGGER fail_monitoring_audit BEFORE INSERT ON events BEGIN SELECT RAISE(ABORT, 'fixture failure'); END")
	}); err != nil {
		t.Fatal(err)
	}
	if s.SetMonitoringSeconds(ctx, 10) == nil {
		t.Fatal("audit failure ignored")
	}
	if got, _ := s.MonitoringSeconds(ctx); got != 60 {
		t.Fatal("partial settings transaction")
	}
}
