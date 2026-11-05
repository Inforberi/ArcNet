package store

import (
	"context"
	"github.com/arcvpn/arcvpn/internal/devices"
	"testing"
)

func TestWireGuardDeviceAllocationAndRestore(t *testing.T) {
	st, u := usageFixture(t)
	ctx := context.Background()
	if err := st.UpsertProtocol(ctx, Protocol{Kind: "wireguard"}); err != nil {
		t.Fatal(err)
	}
	private, public, err := devices.NewWireGuardKeys()
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SaveCredentials(ctx, []ProtocolCredential{{UserID: u.ID, Kind: "wireguard", PrivateKey: private, PublicKey: public, Address: "10.76.255.254"}}); err != nil {
		t.Fatal(err)
	}
	if err := st.ChangeDevices(ctx, u.ID, 0, func(r *devices.Record) error { r.Devices = append(r.Devices, testDevice()); return nil }); err != nil {
		t.Fatal(err)
	}
	before, err := st.UserDevices(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	d := before.Devices[0]
	if d.WGAddress != "10.76.255.253" || devices.ValidateWireGuard(d) != nil || d.WGPrivate == "" {
		t.Fatal("device allocation", d.WGAddress)
	}
	// Old portable state without WireGuard fields must retain existing credentials.
	imported := before
	imported.Devices = append([]devices.Device{}, before.Devices...)
	imported.Devices[0].WGPrivate = ""
	imported.Devices[0].WGPublic = ""
	imported.Devices[0].WGAddress = ""
	state := devices.State{imported}
	creds, _ := st.ListProtocolCredentials(ctx)
	profiles, _ := st.ListProtocols(ctx)
	if err := st.ReplaceFullStateWithDevices(ctx, nil, []User{u}, profiles, creds, nil, nil, nil, nil, nil, &state); err != nil {
		t.Fatal(err)
	}
	got, err := st.UserDevices(ctx, u.ID)
	if err != nil || got.Devices[0].WGPrivate != d.WGPrivate || got.Devices[0].WGAddress != d.WGAddress {
		t.Fatal("legacy restore rotated peer", err)
	}
	// A new legacy credential may choose a default now reserved by a device.
	other := u
	other.ID = 0
	other.Name = "other"
	other.UUID = "44444444-4444-4444-8444-444444444444"
	other.Subscription = "other-token"
	if err := st.InsertUser(ctx, other); err != nil {
		t.Fatal(err)
	}
	other, err = st.ByToken(ctx, "other-token")
	if err != nil {
		t.Fatal(err)
	}
	p2, k2, _ := devices.NewWireGuardKeys()
	if err := st.SaveCredentials(ctx, []ProtocolCredential{{UserID: other.ID, Kind: "wireguard", PrivateKey: p2, PublicKey: k2, Address: d.WGAddress}}); err != nil {
		t.Fatal(err)
	}
	credentials, _ := st.CredentialsForUser(ctx, other.ID)
	if len(credentials) != 1 || credentials[0].Address == d.WGAddress || credentials[0].Address == "10.76.255.254" {
		t.Fatal("address collision")
	}
}
