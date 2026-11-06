package store

import (
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/arcvpn/arcvpn/internal/db"
)

func appliedCascadeProofFixture(t *testing.T) (*cascadeFixture, CascadeDeployment) {
	t.Helper()
	f := newCascadeFixture(t)
	d := f.start(t, f.prepare(t))
	f.ack(t, f.command(t, f.b, f.sb), f.b, f.sb, "applied")
	f.ack(t, f.command(t, f.a, f.sa), f.a, f.sa, "applied")
	return f, f.state(t, d.ID, "awaiting_probe")
}
func cascadeReport(g CascadeProbeGrant) CascadeProbeReport {
	return CascadeProbeReport{DeploymentID: g.DeploymentID, Version: g.Version, EntryRevision: g.EntryRevision, EntryDigest: g.EntryDigest, ExitRevision: g.ExitRevision, ExitDigest: g.ExitDigest, ExitIP: g.ExpectedIP, DNSMode: "doh_via_cascade", Result: "pass", DurationMS: 100}
}
func TestCascadeProbeBoundToBothRevisionsAndExplicitPublication(t *testing.T) {
	f, d := appliedCascadeProofFixture(t)
	if e := f.s.PublishCascade(f.ctx, d.ID, d.Version); !errors.Is(e, ErrConflict) {
		t.Fatal("publication without external proof", e)
	}
	g, e := f.s.CreateCascadeProbeGrant(f.ctx, d.ID, d.Version)
	if e != nil {
		t.Fatal(e)
	}
	report := cascadeReport(g)
	report.ExitDigest = "wrong"
	if e := f.s.CompleteCascadeProbe(f.ctx, g.Token, report); !errors.Is(e, ErrProbeGrant) {
		t.Fatal("wrong exit revision/digest accepted", e)
	}
	report = cascadeReport(g)
	if e := f.s.CompleteCascadeProbe(f.ctx, g.Token, report); e != nil {
		t.Fatal(e)
	}
	if e := f.s.CompleteCascadeProbe(f.ctx, g.Token, report); !errors.Is(e, ErrProbeGrant) {
		t.Fatal("grant replay", e)
	}
	d = f.state(t, d.ID, "verified")
	if nodes, e := f.s.PublishedCascadeEntries(f.ctx); e != nil || len(nodes) != 0 {
		t.Fatal("proof auto-published")
	}
	if e := f.s.PublishCascade(f.ctx, d.ID, d.Version); e != nil {
		t.Fatal(e)
	}
	d = f.state(t, d.ID, "published")
	if nodes, e := f.s.PublishedCascadeEntries(f.ctx); e != nil || len(nodes) != 1 || nodes[0] != f.a {
		t.Fatal("publication missing", e)
	}
	if e := f.s.RollbackCascade(f.ctx, d.ID, d.Version); e != nil {
		t.Fatal(e)
	}
	if nodes, e := f.s.PublishedCascadeEntries(f.ctx); e != nil || len(nodes) != 0 {
		t.Fatal("rollback did not revoke publication")
	}
}
func TestCascadeProbeFailsWrongIPMissingDNSAndStaleEvidence(t *testing.T) {
	for _, kind := range []string{"wrong_ip", "missing_dns", "expired", "changed_revision"} {
		t.Run(kind, func(t *testing.T) {
			f, d := appliedCascadeProofFixture(t)
			g, e := f.s.CreateCascadeProbeGrant(f.ctx, d.ID, d.Version)
			if e != nil {
				t.Fatal(e)
			}
			r := cascadeReport(g)
			switch kind {
			case "wrong_ip":
				r.ExitIP = "8.8.8.8"
			case "missing_dns":
				r.DNSMode = ""
			case "expired":
				if e := f.s.DB.WithTx(f.ctx, func(tx *db.Tx) error {
					return tx.Exec("UPDATE cascade_probe_grants SET expires=? WHERE deployment_id=?", strconv.FormatInt(time.Now().Unix()-1, 10), d.ID)
				}); e != nil {
					t.Fatal(e)
				}
			case "changed_revision":
				if e := f.s.DB.WithTx(f.ctx, func(tx *db.Tx) error {
					return tx.Exec("UPDATE node_control SET desired_digest='changed' WHERE node_id=?", f.b)
				}); e != nil {
					t.Fatal(e)
				}
			}
			e = f.s.CompleteCascadeProbe(f.ctx, g.Token, r)
			if kind == "expired" || kind == "changed_revision" {
				if e == nil {
					t.Fatal("stale grant accepted")
				}
				return
			}
			if e != nil {
				t.Fatal(e)
			}
			d = f.state(t, d.ID, "probe_failed")
			if e := f.s.PublishCascade(f.ctx, d.ID, d.Version); !errors.Is(e, ErrConflict) {
				t.Fatal("failed probe published")
			}
		})
	}
	f, d := appliedCascadeProofFixture(t)
	g, e := f.s.CreateCascadeProbeGrant(f.ctx, d.ID, d.Version)
	if e != nil {
		t.Fatal(e)
	}
	if e := f.s.CompleteCascadeProbe(f.ctx, g.Token, cascadeReport(g)); e != nil {
		t.Fatal(e)
	}
	d = f.state(t, d.ID, "verified")
	if e := f.s.PublishCascade(f.ctx, d.ID, d.Version); e != nil {
		t.Fatal(e)
	}
	if e := f.s.DB.WithTx(f.ctx, func(tx *db.Tx) error {
		return tx.Exec("UPDATE cascade_evidence SET expires=? WHERE deployment_id=?", strconv.FormatInt(time.Now().Unix()-1, 10), d.ID)
	}); e != nil {
		t.Fatal(e)
	}
	if nodes, e := f.s.PublishedCascadeEntries(f.ctx); e != nil || len(nodes) != 0 {
		t.Fatal("expired publication exported")
	}
}
