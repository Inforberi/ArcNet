package store

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/arcvpn/arcvpn/internal/db"
)

func TestLocalRecoveryPreservesMFAAndVPNUnlessExplicit(t *testing.T) {
	s, done := createControlTestStore(t)
	defer done()
	ctx := context.Background()
	if err := s.CreateAdmin(ctx, "admin", "private-admin-hash"); err != nil {
		t.Fatal(err)
	}
	if err := s.DB.WithTx(ctx, func(tx *db.Tx) error {
		return tx.Exec("INSERT INTO admin_mfa(id,secret,pending_secret,pending_until) VALUES(1,'private-mfa-seed','unfinished-enrollment',9999999999)")
	}); err != nil {
		t.Fatal(err)
	}
	user := User{Name: "vpn-user", UUID: "private-vpn-uuid", Subscription: "private-subscription-token", Enabled: true}
	if err := s.InsertUser(ctx, user); err != nil {
		t.Fatal(err)
	}
	if err := s.AddSession(ctx, "private-session", "private-csrf", 9999999999); err != nil {
		t.Fatal(err)
	}
	before, _ := s.ListUsers(ctx)
	if err := s.RecoverAdmin(ctx, "new-hash", false); err != nil {
		t.Fatal(err)
	}
	if active, _ := s.MFAStatus(ctx); !active {
		t.Fatal("password reset disabled MFA")
	}
	if err := s.DB.Read(ctx, func(tx *db.Tx) error {
		rows, err := tx.Query("SELECT pending_secret,pending_until FROM admin_mfa")
		if err != nil {
			return err
		}
		if rows[0]["pending_secret"] != "" || rows[0]["pending_until"] != "0" {
			t.Fatal("unfinished MFA enrollment survived recovery")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, valid, _ := s.Session(ctx, "private-session"); valid {
		t.Fatal("session remained active")
	}
	after, _ := s.ListUsers(ctx)
	a, _ := json.Marshal(before)
	b, _ := json.Marshal(after)
	if string(a) != string(b) || len(after) != 1 || after[0].Subscription != before[0].Subscription {
		t.Fatal("recovery changed VPN state")
	}
	settings, err := s.RecoverySettings(ctx)
	if err != nil || settings.Username != "admin" || !settings.MFAEnabled || settings.ActiveSessions != 0 {
		t.Fatalf("settings: %#v %v", settings, err)
	}
	blob, _ := json.Marshal(settings)
	events, _ := s.Events(ctx)
	audit, _ := json.Marshal(events)
	for _, private := range []string{"new-hash", "private-admin-hash", "private-mfa-seed", "private-vpn-uuid", "private-subscription-token", "private-session", "private-csrf"} {
		if strings.Contains(string(blob)+string(audit), private) {
			t.Fatal("settings/audit exposed secret")
		}
	}
	if err := s.RecoverAdmin(ctx, "next-hash", true); err != nil {
		t.Fatal(err)
	}
	if active, _ := s.MFAStatus(ctx); active {
		t.Fatal("explicit MFA recovery did not clear it")
	}
}

func TestRecoveryRollsBackPasswordMFAAndSessionsOnAuditFailure(t *testing.T) {
	s, done := createControlTestStore(t)
	defer done()
	ctx := context.Background()
	if err := s.CreateAdmin(ctx, "admin", "old-hash"); err != nil {
		t.Fatal(err)
	}
	if err := s.AddSession(ctx, "session", "csrf", 9999999999); err != nil {
		t.Fatal(err)
	}
	err := s.DB.WithTx(ctx, func(tx *db.Tx) error {
		if err := tx.Exec("INSERT INTO admin_mfa(id,secret) VALUES(1,'seed')"); err != nil {
			return err
		}
		return tx.Exec("CREATE TRIGGER fail_recovery_audit BEFORE INSERT ON events BEGIN SELECT RAISE(ABORT,'fixture failure'); END")
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.RecoverAdmin(ctx, "new-hash", true); err == nil {
		t.Fatal("failed audit committed recovery")
	}
	if hash, _ := s.AdminHash(ctx, "admin"); hash != "old-hash" {
		t.Fatal("partial password change")
	}
	if active, _ := s.MFAStatus(ctx); !active {
		t.Fatal("partial MFA clear")
	}
	if _, valid, _ := s.Session(ctx, "session"); !valid {
		t.Fatal("partial session revocation")
	}
}

func TestLocalSessionRevocationDoesNotChangePasswordOrMFA(t *testing.T) {
	s, done := createControlTestStore(t)
	defer done()
	ctx := context.Background()
	if err := s.CreateAdmin(ctx, "admin", "hash"); err != nil {
		t.Fatal(err)
	}
	if err := s.AddSession(ctx, "session", "csrf", 9999999999); err != nil {
		t.Fatal(err)
	}
	if err := s.RevokeAdminSessions(ctx); err != nil {
		t.Fatal(err)
	}
	if hash, _ := s.AdminHash(ctx, "admin"); hash != "hash" {
		t.Fatal("session revocation changed password")
	}
	if _, valid, _ := s.Session(ctx, "session"); valid {
		t.Fatal("session survived revocation")
	}
}
