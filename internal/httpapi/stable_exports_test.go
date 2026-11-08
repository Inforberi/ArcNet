package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/arcvpn/arcvpn/internal/app"
	"github.com/arcvpn/arcvpn/internal/protocols"
)

func TestStableExportDiscardsResponseAcrossPublication(t *testing.T) {
	s, _ := firstProtocolServer(t)
	ctx := context.Background()
	if err := s.App.Setup(ctx, "198.51.100.1", "www.example.com", 443); err != nil {
		t.Fatal(err)
	}
	r, _ := s.App.Store.ReadReality(ctx)
	fp := "firefox"
	handler := s.stableExports(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("candidate-credential"))
		if err := s.App.EditConnection(ctx, "vless-reality", app.ConnectionRevision(app.RealityProfile(r)), app.ConnectionSettings{Port: r.Port, SNI: r.SNI, Target: r.Target, Enabled: r.Enabled, Fingerprint: &fp}); err != nil {
			t.Error(err)
		}
	})
	w := httptest.NewRecorder()
	handler(w, httptest.NewRequest("GET", "/sub/fixture", nil))
	if w.Code != 503 || strings.Contains(w.Body.String(), "candidate-credential") || w.Header().Get("Retry-After") != "1" {
		t.Fatal("intermediate export escaped", w.Code)
	}
}
func TestStableExportBlockedDuringRotation(t *testing.T) {
	s, _ := firstProtocolServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p, err := s.App.AddProtocol(ctx, protocols.WireGuard, "198.51.100.1", 0, "", "")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := s.App.PreviewKeyRotation(ctx, p.Kind)
	if err != nil {
		t.Fatal(err)
	}
	entered, release, done := make(chan struct{}, 1), make(chan struct{}), make(chan error, 1)
	s.App.RuntimeCheck = func() error {
		select {
		case entered <- struct{}{}:
		default:
		}
		<-release
		return nil
	}
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	go func() { done <- s.App.RotateConnectionKeys(ctx, p.Kind, plan.Revision, plan.Confirmation) }()
	select {
	case <-entered:
	case err = <-done:
		t.Fatal("rotation exited early", err)
	case <-time.After(3 * time.Second):
		t.Fatal("rotation did not reach gate")
	}
	called := false
	handler := s.stableExports(func(w http.ResponseWriter, _ *http.Request) { called = true; _, _ = w.Write([]byte("credential")) })
	w := httptest.NewRecorder()
	handler(w, httptest.NewRequest("GET", "/sub/fixture", nil))
	if w.Code != 503 || called {
		t.Fatal("rotation allowed export", w.Code)
	}
	close(release)
	if err = <-done; err != nil {
		t.Fatal(err)
	}
}
