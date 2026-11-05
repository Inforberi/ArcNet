package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/arcvpn/arcvpn/internal/db"
	"github.com/arcvpn/arcvpn/internal/mfa"
)

func TestMFALifecycleAndLocalRecovery(t *testing.T) {
	ctx := context.Background()
	d, err := db.Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	s := &Store{DB: d}
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateAdmin(ctx, "admin", "dummyhash"); err != nil {
		t.Fatal(err)
	}
	enabled, err := s.MFAStatus(ctx)
	if err != nil || enabled {
		t.Fatalf("default enabled: %v %v", enabled, err)
	}
	used, err := s.CheckLoginMFA(ctx, "", time.Now())
	if err != nil || !used {
		t.Fatal("login without MFA failed")
	}
	secret, err := mfa.NewSecret()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1800000060, 0)
	if err = s.BeginMFA(ctx, secret, now.Add(10*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err = s.BeginMFA(ctx, secret, now.Add(10*time.Minute)); err != nil {
		t.Fatal("should allow restarting pending setup", err)
	}
	code, _ := mfa.Code(secret, now.Unix()/30, 6)
	if err = s.ConfirmMFA(ctx, "000000", now); err == nil && code != "000000" {
		t.Fatal("accepted incorrect confirmation")
	}
	if err = s.ConfirmMFA(ctx, code, now); err != nil {
		t.Fatal(err)
	}
	enabled, err = s.MFAStatus(ctx)
	if err != nil || !enabled {
		t.Fatal("confirmation not persistent")
	}
	if err = s.BeginMFA(ctx, secret, now.Add(time.Minute)); !errors.Is(err, ErrMFAEnabled) {
		t.Fatal("second MFA setup accepted", err)
	}
	accepted, err := s.CheckLoginMFA(ctx, code, now)
	if err != nil || accepted {
		t.Fatal("replay accepted")
	}
	next := now.Add(30 * time.Second)
	nextCode, _ := mfa.Code(secret, next.Unix()/30, 6)
	accepted, err = s.CheckLoginMFA(ctx, nextCode, next)
	if err != nil || !accepted {
		t.Fatal("valid login code rejected", err)
	}
	accepted, err = s.CheckLoginMFA(ctx, nextCode, next)
	if err != nil || accepted {
		t.Fatal("used login code accepted", err)
	}
	later := now.Add(60 * time.Second)
	laterCode, _ := mfa.Code(secret, later.Unix()/30, 6)
	if err = s.DisableMFA(ctx, laterCode, later); err != nil {
		t.Fatal(err)
	}
	enabled, err = s.MFAStatus(ctx)
	if err != nil || enabled {
		t.Fatal("MFA still enabled")
	}
	if err = s.BeginMFA(ctx, secret, later.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	confirmCode, _ := mfa.Code(secret, later.Unix()/30, 6)
	if err = s.ConfirmMFA(ctx, confirmCode, later); err != nil {
		t.Fatal(err)
	}
	if err = s.ResetAdminPassword(ctx, "newhash"); err != nil {
		t.Fatal(err)
	}
	enabled, err = s.MFAStatus(ctx)
	if err != nil || enabled {
		t.Fatal("recovery did not disable MFA")
	}
}
func TestMFAPendingExpiry(t *testing.T) {
	ctx := context.Background()
	d, err := db.Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	s := &Store{DB: d}
	if err = s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	secret, err := mfa.NewSecret()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1800000060, 0)
	if err = s.BeginMFA(ctx, secret, now.Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	code, _ := mfa.Code(secret, now.Unix()/30, 6)
	if err = s.ConfirmMFA(ctx, code, now); !errors.Is(err, ErrInvalidOTP) {
		t.Fatal("expired token accepted")
	}
}
