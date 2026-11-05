package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arcvpn/arcvpn/internal/db"
	"github.com/arcvpn/arcvpn/internal/engines"
	"github.com/arcvpn/arcvpn/internal/protocols"
	"github.com/arcvpn/arcvpn/internal/routes"
	"github.com/arcvpn/arcvpn/internal/store"
	"github.com/arcvpn/arcvpn/internal/xray"
)

func TestStage2LifecycleAndPortableRestore(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	database, e := db.Open(filepath.Join(dir, "state.sqlite"))
	if e != nil {
		t.Fatal(e)
	}
	defer database.Close()
	st := &store.Store{DB: database}
	if e = st.Migrate(ctx); e != nil {
		t.Fatal(e)
	}
	fakeXray := filepath.Join(dir, "xray")
	script := `#!/bin/sh
if [ "$1" = "x25519" ];then
 echo 'PrivateKey: AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA'
 echo 'Password: BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB'
 exit 0
fi
if [ "$1" = "run" ];then exit 0;fi
exit 1
`
	if e = os.WriteFile(fakeXray, []byte(script), 0755); e != nil {
		t.Fatal(e)
	}
	manager := &engines.Manager{Dir: filepath.Join(dir, "engines"), SkipRestart: true}
	a := &App{Store: st, Core: xray.Controller{Binary: fakeXray, Path: filepath.Join(dir, "xray.json"), SkipRestart: true}, Engines: manager}
	if e = a.Setup(ctx, "198.51.100.88", "www.example.com", 443); e != nil {
		t.Fatal(e)
	}
	alice, e := a.AddUser(ctx, "Alice")
	if e != nil {
		t.Fatal(e)
	}
	bob, e := a.AddUser(ctx, "Bob")
	if e != nil {
		t.Fatal(e)
	}
	for _, kind := range protocols.Kinds() {
		sni, target := "", ""
		if kind == protocols.XHTTP {
			sni = "www.example.com"
			target = "www.example.com"
		}
		_, e = a.AddProtocol(ctx, kind, "198.51.100.88", 0, sni, target)
		if e != nil {
			t.Fatalf("create %s: %v", kind, e)
		}
	}
	profiles, e := st.ListProtocols(ctx)
	if e != nil || len(profiles) != 4 {
		t.Fatalf("profiles: %v %+v", e, profiles)
	}
	creds, e := st.ListProtocolCredentials(ctx)
	if e != nil || len(creds) != 8 {
		t.Fatalf("credentials: %v %d", e, len(creds))
	}
	links, e := a.SubscriptionLinks(ctx, alice)
	if e != nil || len(links) != 3 {
		t.Fatalf("subscription list: %v %+v", e, links)
	}
	if !strings.Contains(strings.Join(links, "\n"), "hysteria2://") {
		t.Fatal("Hysteria missing from subscription")
	}
	connections, e := a.Connections(ctx, bob.ID)
	if e != nil || len(connections) != 4 {
		t.Fatalf("Bob connections: %v %d", e, len(connections))
	}
	for _, c := range connections {
		if (c.Kind == protocols.WireGuard || c.Kind == protocols.AmneziaWG) && !strings.Contains(c.Configuration, "PrivateKey = ") {
			t.Fatal("tunnel private key missing")
		}
	}
	for _, name := range []string{"arcwg.conf", "arcawg.conf", "hysteria.yaml"} {
		data, e := os.ReadFile(filepath.Join(manager.Dir, name))
		if e != nil || len(data) < 100 {
			t.Fatalf("engine config %s: %v", name, e)
		}
	}
	draft := routes.DefaultPolicy()
	draft.Name = "Portable direct draft"
	route, e := a.SaveRoute(ctx, draft, 0)
	if e != nil {
		t.Fatal(e)
	}
	saved, e := a.ExportState(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if e = saved.Validate(); e != nil {
		t.Fatal(e)
	}
	if len(saved.Protocols) != 4 || len(saved.Credentials) != 8 {
		t.Fatal("backup missing protocol state")
	}
	if len(saved.Routes) != 1 || saved.Routes[0].ID != route.ID {
		t.Fatal("backup missing route drafts")
	}
	route.Name = "Changed after export"
	if _, e = a.SaveRoute(ctx, route, route.Version); e != nil {
		t.Fatal(e)
	}
	if e = a.ToggleUser(ctx, bob.ID, false); e != nil {
		t.Fatal(e)
	}
	reduced, e := a.Connections(ctx, bob.ID)
	if e != nil || len(reduced) != 0 {
		t.Fatal("disabled user still receives configuration")
	}
	if e = a.SetProtocolEnabled(ctx, protocols.Hysteria2, false); e != nil {
		t.Fatal(e)
	}
	links, e = a.SubscriptionLinks(ctx, alice)
	if e != nil || len(links) != 2 {
		t.Fatal("disabled Hysteria must leave subscription")
	}
	if e = a.RestoreState(ctx, saved); e != nil {
		t.Fatal(e)
	}
	restoredRoute, e := st.RoutePolicy(ctx, route.ID)
	if e != nil || restoredRoute.Name != "Portable direct draft" || restoredRoute.Version <= route.Version {
		t.Fatalf("route restore/CAS failed: %+v %v", restoredRoute, e)
	}
	restored, e := st.ListProtocolCredentials(ctx)
	if e != nil || len(restored) != 8 {
		t.Fatalf("restored credentials: %v %d", e, len(restored))
	}
	after, e := a.SubscriptionLinks(ctx, alice)
	if e != nil || len(after) != 3 {
		t.Fatal("subscription not restored")
	}
	// Importing a legacy v1 archive must stop and clear all obsolete engines.
	legacy := saved
	legacy.Version = 1
	legacy.Usage = nil
	legacy.Protocols = nil
	legacy.Credentials = nil
	legacy.Nodes = nil
	legacy.LogBudgetMB = nil
	legacy.Routes = nil
	if e = a.RestoreState(ctx, legacy); e != nil {
		t.Fatalf("legacy restore: %v", e)
	}
	for _, name := range []string{"arcwg.conf", "arcawg.conf", "hysteria.yaml"} {
		if _, e := os.Stat(filepath.Join(manager.Dir, name)); !os.IsNotExist(e) {
			t.Fatalf("legacy import left stale secret config %s: %v", name, e)
		}
	}
	if e = a.RestoreState(ctx, saved); e != nil {
		t.Fatalf("v2 re-import: %v", e)
	}
	// Failed user deletion must restore per-protocol credentials, not only user row.
	if e = os.WriteFile(fakeXray, []byte("#!/bin/sh\nexit 1\n"), 0755); e != nil {
		t.Fatal(e)
	}
	if e = a.DeleteUser(ctx, alice.ID); e == nil {
		t.Fatal("fake failing Xray did not reject user deletion")
	}
	beforeFailedRestore, e := st.RoutePolicy(ctx, route.ID)
	if e != nil {
		t.Fatal(e)
	}
	badRestore := saved.WithRoutes(saved.Routes)
	badRestore.Routes[0].Name = "Must roll back"
	if e = a.RestoreState(ctx, badRestore); e == nil {
		t.Fatal("failing engine allowed restore")
	}
	afterFailedRestore, e := st.RoutePolicy(ctx, route.ID)
	if e != nil || afterFailedRestore.Name != beforeFailedRestore.Name {
		t.Fatalf("failed restore lost routes: %+v %v", afterFailedRestore, e)
	}
	recoveredCreds, e := st.CredentialsForUser(ctx, alice.ID)
	if e != nil || len(recoveredCreds) != 4 {
		t.Fatalf("deletion rollback lost credentials: %v %d", e, len(recoveredCreds))
	}
	if e = os.WriteFile(fakeXray, []byte(script), 0755); e != nil {
		t.Fatal(e)
	}
	if e = a.SetProtocolEnabled(ctx, protocols.Hysteria2, false); e != nil {
		t.Fatal(e)
	}
	if e = a.DeleteProtocol(ctx, protocols.Hysteria2); e != nil {
		t.Fatal(e)
	}
	if _, e = st.GetProtocol(ctx, protocols.Hysteria2); e != store.ErrNotFound {
		t.Fatalf("deleted profile still exists: %v", e)
	}
}
