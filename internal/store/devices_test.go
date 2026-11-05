package store

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/arcvpn/arcvpn/internal/devices"
)

func testDevice() devices.Device {
	return devices.Device{ID: "22222222-2222-4222-8222-222222222222", UUID: "33333333-3333-4333-8333-333333333333", Token: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", Name: "Phone", CreatedAt: 100}
}

func TestDeviceCASRevocationAndLegacyRestore(t *testing.T) {
	s, u := usageFixture(t)
	ctx := context.Background()
	d := testDevice()
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- s.ChangeDevices(ctx, u.ID, 0, func(r *devices.Record) error { r.Limit = 1; r.Devices = append(r.Devices, d); return nil })
		}()
	}
	wg.Wait()
	close(results)
	success, conflict := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, ErrConflict) {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatal(success, conflict)
	}
	before, err := s.DeviceState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.DeviceByToken(ctx, d.Token); err != nil {
		t.Fatal(err)
	}
	if err := s.ChangeDevices(ctx, u.ID, 1, func(r *devices.Record) error { r.Devices[0].RevokedAt = 200; return nil }); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.DeviceByToken(ctx, d.Token); !errors.Is(err, ErrNotFound) {
		t.Fatal("revoked token accepted", err)
	}
	// Stale v7 cannot resurrect a credential; v1–v6 cannot restore common access.
	if err := s.ReplaceFullStateWithDevices(ctx, nil, []User{u}, nil, nil, nil, nil, nil, nil, nil, &before); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceFullStateWithUsage(ctx, nil, []User{u}, nil, nil, nil, nil, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	got, err := s.UserDevices(ctx, u.ID)
	if err != nil || got.Version < 2 || len(got.Devices) != 1 || got.Devices[0].RevokedAt != 200 {
		t.Fatal("restore lost retirement", got, err)
	}
	if _, _, err := s.DeviceByToken(ctx, d.Token); !errors.Is(err, ErrNotFound) {
		t.Fatal("restore resurrected token", err)
	}
}

func TestDevicesRejectUnsupportedAndRollbackDeletion(t *testing.T) {
	s, u := usageFixture(t)
	ctx := context.Background()
	d := testDevice()
	if err := s.ChangeDevices(ctx, u.ID, 0, func(r *devices.Record) error { r.Devices = []devices.Device{d}; return nil }); err != nil {
		t.Fatal(err)
	}
	state, _ := s.UserDevices(ctx, u.ID)
	accounting, _ := s.UsageBackup(ctx)
	if err := s.DeleteUser(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.DeviceByToken(ctx, d.Token); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	if err := s.RestoreDeletedUser(ctx, u, nil, accounting, state); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.DeviceByToken(ctx, d.Token); err != nil {
		t.Fatal("compensation lost token", err)
	}
	yes, err := s.HasRestrictedPackages(ctx, true)
	if err != nil || !yes {
		t.Fatal("managed devices did not guard aux deployment", yes, err)
	}
	other := testDevice()
	other.ID = "44444444-4444-4444-8444-444444444444"
	other.UUID = "55555555-5555-4555-8555-555555555555"
	other.Token = "BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB"
	if err := s.ChangeDevices(ctx, u.ID, state.Version, func(r *devices.Record) error { r.Devices = append(r.Devices, other); r.Limit = 1; return nil }); !errors.Is(err, devices.ErrInvalid) {
		t.Fatal("transaction-time limit bypass", err)
	}
}
