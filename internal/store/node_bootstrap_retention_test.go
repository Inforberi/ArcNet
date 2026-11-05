package store

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/arcvpn/arcvpn/internal/db"
)

func TestSSHJournalRetentionPreservesActiveInstallMarker(t *testing.T) {
	ctx := context.Background()
	s, closeDB := createControlTestStore(t)
	defer closeDB()
	id, _ := createControlTestNode(t, s)
	original, err := s.CreateNodeBootstrap(ctx, "installed", "9.9.9.9", 22, 443)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AttachNodeBootstrap(ctx, original.ID, id); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateNodeBootstrap(ctx, original.ID, "finished", "installed", ""); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Unix()
	err = s.DB.WithTx(ctx, func(tx *db.Tx) error {
		// Age out the origin entry. It is still required to validate the
		// managed marker before a future SSH update or uninstall.
		if err := tx.Exec("UPDATE node_bootstrap_operations SET created_at=?,updated_at=? WHERE id=?",
			strconv.FormatInt(now-600*86400, 10), strconv.FormatInt(now-600*86400, 10), original.ID); err != nil {
			return err
		}
		for i := 0; i < 590; i++ {
			opID := fmt.Sprintf("expired-%04d", i)
			date := strconv.FormatInt(now-int64(590-i)*86400, 10)
			if err := tx.Exec(`INSERT INTO node_bootstrap_operations(id,name,address,ssh_port,vpn_port,status,step,created_at,updated_at,operation_type)
                VALUES(?,'historical','9.9.9.9',22,443,'failed','preflight',?,?,'install')`, opID, date, date); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateNodeBootstrap(ctx, "new", "9.9.9.9", 22, 443); err != nil {
		t.Fatal(err)
	}
	verified, err := s.InstalledBootstrapForNode(ctx, id)
	if err != nil || verified.ID != original.ID {
		t.Fatalf("active managed origin was pruned: %+v, %v", verified, err)
	}
	err = s.DB.Read(ctx, func(tx *db.Tx) error {
		rows, err := tx.Query("SELECT id FROM node_bootstrap_operations")
		if err != nil {
			return err
		}
		// The last 90 days of history are retained even when there are
		// more than 50 entries, plus the original managed installation.
		if len(rows) < 90 || len(rows) > 92 {
			t.Errorf("unbounded or missing SSH journal history: %d", len(rows))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
