package store

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/arcvpn/arcvpn/internal/routingprofile"
	"github.com/arcvpn/arcvpn/internal/targetcheck"
)

func TestRoutingProfileCASRestoreAndLegacyPreservation(t *testing.T) {
	s, done := createControlTestStore(t)
	defer done()
	ctx := context.Background()
	p := routingprofile.Default()
	p.Name = "Правила"
	p.Direct = []string{"example.com"}
	saved, err := s.SaveRoutingProfile(ctx, p, 0)
	if err != nil || saved.ID == "" || saved.Version != 1 {
		t.Fatalf("save: %#v %v", saved, err)
	}
	p = saved
	p.Name = "Изменено"
	current, err := s.SaveRoutingProfile(ctx, p, p.Version)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SaveRoutingProfile(ctx, p, p.Version); !errors.Is(err, ErrConflict) {
		t.Fatal("stale form overwrote newer revision")
	}
	if err = s.ReplaceFullStateWithRoutes(ctx, nil, nil, nil, nil, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	profiles, _ := s.RoutingProfiles(ctx)
	if len(profiles) != 1 || profiles[0].Version != current.Version {
		t.Fatal("legacy import erased profiles")
	}
	restored := []routingprofile.Profile{saved}
	if err = s.ReplaceFullStateWithProfiles(ctx, nil, nil, nil, nil, nil, nil, nil, &restored); err != nil {
		t.Fatal(err)
	}
	profiles, _ = s.RoutingProfiles(ctx)
	if len(profiles) != 1 || profiles[0].Version <= current.Version {
		t.Fatal("restore did not invalidate stale forms")
	}
	bad := []routingprofile.Profile{saved, saved}
	if err = s.ReplaceFullStateWithProfiles(ctx, nil, nil, nil, nil, nil, nil, nil, &bad); err == nil {
		t.Fatal("duplicate imported")
	}
	again, _ := s.RoutingProfiles(ctx)
	if len(again) != 1 || again[0].Version != profiles[0].Version {
		t.Fatal("failed import partially changed state")
	}
	if err = s.DeleteRoutingProfile(ctx, saved.ID, saved.Version); !errors.Is(err, ErrConflict) {
		t.Fatal("stale delete accepted")
	}
	if err = s.DeleteRoutingProfile(ctx, again[0].ID, again[0].Version); err != nil {
		t.Fatal(err)
	}
}

func TestCatalogRejectsReplayAcrossManualReplacement(t *testing.T) {
	s, done := createControlTestStore(t)
	defer done()
	ctx := context.Background()
	signed := targetcheck.DefaultCatalog()
	signed.FeedVersion = 7
	signed.Source = "https://example.com/list"
	signed.ExpiresAt = time.Now().Add(time.Hour).Unix()
	if err := s.SaveTargetCatalog(ctx, signed, 1); err != nil {
		t.Fatal(err)
	}
	old, _ := s.TargetCatalog(ctx)
	manual := targetcheck.DefaultCatalog()
	manual.Source = "manual"
	if err := s.SaveTargetCatalog(ctx, manual, old.Version); err != nil {
		t.Fatal(err)
	}
	old, _ = s.TargetCatalog(ctx)
	if err := s.SaveTargetCatalog(ctx, signed, old.Version); !errors.Is(err, ErrConflict) {
		t.Fatal("signed replay accepted after manual replacement")
	}
	signed.FeedVersion = 8
	if err := s.SaveTargetCatalog(ctx, signed, old.Version); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveTargetCatalog(ctx, manual, old.Version); !errors.Is(err, ErrConflict) {
		t.Fatal("catalogue CAS bypassed")
	}
}

func TestTargetReportRecomputesRecommendationAndNeverGrantsVPNProof(t *testing.T) {
	candidates := []targetcheck.Candidate{{Host: "fast.example.com", SNI: "fast.example.com"}, {Host: "stable.example.com", SNI: "stable.example.com"}}
	job := TargetJob{NodeID: "node-a", Candidates: candidates}
	samples := []targetcheck.Measurement{{TCPMS: 10, TLSMS: 5}, {TCPMS: 11, TLSMS: 5}, {TCPMS: 12, TLSMS: 5}}
	forged := targetcheck.Report{MeasuredFrom: "independent-client", VPNVerified: true, Recommended: &candidates[0], Results: []targetcheck.Result{
		{Candidate: candidates[0], Eligible: true, Successes: 3, MedianMS: 1, HTTPVerified: true, HTTPStatus: 302, Samples: []targetcheck.Measurement{{TCPMS: 1}, {TCPMS: 1}, {TCPMS: 1}}, VPNVerified: true},
		{Candidate: candidates[1], HTTPVerified: true, HTTPStatus: 200, Samples: samples, VPNVerified: true},
	}}
	report, err := validateTargetReport(job, forged)
	if err != nil || report.MeasuredFrom != "node-a" || report.VPNVerified || report.Recommended == nil || *report.Recommended != candidates[1] || report.Results[0].Eligible || report.Results[1].VPNVerified || report.Results[1].MedianMS != 16 {
		t.Fatalf("forged counters/proof trusted: %#v %v", report, err)
	}
	report.Results[1].Samples[0].TCPMS = math.NaN()
	if _, err = validateTargetReport(job, report); err == nil {
		t.Fatal("non-finite measurement accepted")
	}
}

func TestTargetJobsCapabilityBindingLeaseAndReplay(t *testing.T) {
	s, done := createControlTestStore(t)
	defer done()
	ctx := context.Background()
	node, secret := createControlTestNode(t, s)
	candidates := []targetcheck.Candidate{{Host: "example.com", SNI: "example.com"}}
	if _, err := s.QueueTargetCheck(ctx, node, candidates); !errors.Is(err, ErrIncompatibleNodeProtocol) {
		t.Fatal("old agent accepted a diagnostic job")
	}
	if err := s.TargetHello(ctx, node, secret); err != nil {
		t.Fatal(err)
	}
	job, err := s.QueueTargetCheck(ctx, node, candidates)
	if err != nil {
		t.Fatal(err)
	}
	if reused, e := s.QueueTargetCheck(ctx, node, candidates); e != nil || !reused.Reused || reused.ID != job.ID {
		t.Fatal("identical check was not reused", e)
	}
	if _, err = s.QueueTargetCheck(ctx, node, []targetcheck.Candidate{{Host: "other.example.com", SNI: "other.example.com"}}); !errors.Is(err, ErrNodeBusy) {
		t.Fatal("different overlapping job allowed")
	}
	if _, err = s.PollTargetCheck(ctx, node, "foreign-secret"); !errors.Is(err, ErrNodeUnauthorized) {
		t.Fatal("foreign agent polled job")
	}
	leased, err := s.PollTargetCheck(ctx, node, secret)
	if err != nil || leased == nil || leased.ID != job.ID || leased.Version != 1 {
		t.Fatalf("lease: %#v %v", leased, err)
	}
	if next, err := s.PollTargetCheck(ctx, node, secret); err != nil || next != nil {
		t.Fatal("lease ignored")
	}
	report := targetcheck.Report{Results: []targetcheck.Result{{Candidate: candidates[0], Error: "fixture unavailable"}}}
	if err = s.CompleteTargetCheck(ctx, job.ID, node, "foreign-secret", job.Digest, report); !errors.Is(err, ErrNodeUnauthorized) {
		t.Fatal("foreign report accepted")
	}
	if err = s.CompleteTargetCheck(ctx, job.ID, node, secret, "wrong-digest", report); err == nil {
		t.Fatal("wrong digest accepted")
	}
	if err = s.CompleteTargetCheck(ctx, job.ID, node, secret, job.Digest, report); err != nil {
		t.Fatal(err)
	}
	if err = s.CompleteTargetCheck(ctx, job.ID, node, secret, job.Digest, targetcheck.Report{}); err != nil {
		t.Fatal("lost ACK retry is not idempotent")
	}
	stored, err := s.TargetCheck(ctx, job.ID)
	if err != nil || stored.State != "done" || stored.Report == nil || len(stored.Report.Results) != 1 || stored.Report.Recommended != nil || stored.Report.VPNVerified {
		t.Fatalf("first report changed or false proof: %#v %v", stored, err)
	}
}
