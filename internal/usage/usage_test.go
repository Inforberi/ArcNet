package usage

import "testing"

func TestCounterReplayRestartAndRegression(t *testing.T) {
	c := Counter{UserID: 1, Identity: "11111111-1111-4111-8111-111111111111", Stream: "vless-reality", Epoch: "a", StartedAt: 10, ObservedAt: 100, Upload: 100, Download: 200}
	up, down, err := Delta(nil, c)
	if err != nil || up != 100 || down != 200 {
		t.Fatal(up, down, err)
	}
	up, down, err = Delta(&c, c)
	if err != nil || up+down != 0 {
		t.Fatal("replay charged")
	}
	next := c
	next.ObservedAt++
	next.Upload += 50
	up, down, err = Delta(&c, next)
	if err != nil || up != 50 || down != 0 {
		t.Fatal(up, down, err)
	}
	next.Upload = 1
	if _, _, err := Delta(&c, next); err == nil {
		t.Fatal("unexplained reset accepted")
	}
	next.Epoch = "b"
	next.StartedAt++
	up, down, err = Delta(&c, next)
	if err != nil || up != 1 || down != 200 {
		t.Fatal("restart not accounted", up, down, err)
	}
	retired := c
	retired.ObservedAt = 200
	if _, _, err := Delta(&next, retired); err == nil {
		t.Fatal("retired epoch replay accepted")
	}
	next.Identity = "22222222-2222-4222-8222-222222222222"
	if _, _, err := Delta(&c, next); err == nil {
		t.Fatal("identity change accepted")
	}
}
func TestPackageExpiryQuotaAndMissingSourceAreSeparate(t *testing.T) {
	p := Package{UserID: 1, Period: 1}
	if p.Reason(100, false) != "" {
		t.Fatal("unlimited depends on counters")
	}
	p.QuotaBytes = 100
	if p.Reason(100, false) != "accounting_unavailable" {
		t.Fatal("unknown source allowed limited access")
	}
	p.Upload = 60
	p.Download = 40
	if p.Reason(100, true) != "quota_exhausted" {
		t.Fatal("both directions not counted")
	}
	p.ExpiresAt = 100
	if p.Reason(100, true) != "expired" {
		t.Fatal("expiry boundary")
	}
	p.Download = MaxBytes
	if p.Validate() == nil {
		t.Fatal("overflow accepted")
	}
}
