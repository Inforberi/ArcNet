package usage

import "testing"

func TestNodeCounterSourceIsolatesEpochsAndPortableKeys(t *testing.T) {
	c := Counter{UserID: 1, Identity: "11111111-1111-4111-8111-111111111111", Stream: "vless-reality", Epoch: "same", StartedAt: 1, ObservedAt: 1, Upload: 10}
	remote := c
	remote.Source = "22222222-2222-4222-8222-222222222222"
	if remote.Validate() != nil || remote.StorageStream() == c.StorageStream() {
		t.Fatal("source collision")
	}
	if _, _, e := Delta(&c, remote); e == nil {
		t.Fatal("cross-node delta accepted")
	}
	state := State{Checkpoints: []Counter{c, remote}}
	if e := state.Validate(map[int64]string{1: c.Identity}); e != nil {
		t.Fatal(e)
	}
	remote.Source = "../foreign"
	if remote.Validate() == nil {
		t.Fatal("invalid source accepted")
	}
}
