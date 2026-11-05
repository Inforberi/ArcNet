package backup

import (
	"github.com/arcvpn/arcvpn/internal/usage"
	"testing"
	"time"
)

func TestV6PreservesPackagesAndRejectsInvalidAccounting(t *testing.T) {
	s := sample()
	s.Version = 6
	u := s.Users[0]
	s = s.WithUsage(usage.State{Packages: []usage.Package{{UserID: u.ID, QuotaBytes: 1000, Upload: 100, Download: 200, Period: 1, Version: 1}}, Checkpoints: []usage.Counter{{UserID: u.ID, Identity: u.UUID, Stream: "vless-reality", Epoch: "fixture", StartedAt: 1, ObservedAt: 100, Upload: 100, Download: 200}}, Debits: []usage.Debit{{UserID: u.ID, Period: 1, At: 60, Upload: 100, Download: 200}}})
	blob, err := Encrypt(s, "backup-password-long-enough")
	if err != nil {
		t.Fatal(err)
	}
	restored, err := Decrypt(blob, "backup-password-long-enough")
	if err != nil || restored.Version != 6 || restored.Usage == nil || restored.Usage.Packages[0].Download != 200 || len(restored.Usage.Checkpoints) != 1 {
		t.Fatal("accounting lost", err)
	}
	s.Version = 5
	if s.Validate() == nil {
		t.Fatal("legacy accepted new usage fields")
	}
	s.Version = 6
	s.Usage.Checkpoints[0].Identity = "22222222-2222-4222-8222-222222222222"
	if s.Validate() == nil {
		t.Fatal("foreign counters accepted")
	}
}

func TestV6BoundedDebitHistoryFitsEncryptedArchive(t *testing.T) {
	s := sample()
	u := s.Users[0]
	state := usage.State{Packages: []usage.Package{{UserID: u.ID, QuotaBytes: 1_000_000_000, Upload: 65536 * 1000, Period: 1, Version: 1}}}
	now := time.Now().Unix() / 60 * 60
	for i := 0; i < 65536; i++ {
		state.Debits = append(state.Debits, usage.Debit{UserID: u.ID, Period: 1, At: now - int64(i)*60, Upload: 1000})
	}
	s = s.WithUsage(state)
	blob, err := Encrypt(s, "backup-password-long-enough")
	if err != nil {
		t.Fatal(err)
	}
	restored, err := Decrypt(blob, "backup-password-long-enough")
	if err != nil || restored.Usage == nil || len(restored.Usage.Debits) != 65536 {
		t.Fatal("bounded history did not round-trip", err)
	}
	t.Logf("65536 debit rows: encrypted archive %d bytes", len(blob))
}
