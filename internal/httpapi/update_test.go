package httpapi

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/arcvpn/arcvpn/internal/auth"
	"github.com/arcvpn/arcvpn/internal/ops"
	"github.com/arcvpn/arcvpn/internal/updater"
)

type updateFixture struct {
	payload     []byte
	s           *Server
	cookie      *http.Cookie
	downloading chan struct{}
	proceed     chan struct{}
	fail        func()
}

func onlineUpdateFixture(t *testing.T) updateFixture {
	t.Helper()
	s, cookie := monitoringFixture(t)
	dir := t.TempDir()
	pub, key, _ := ed25519.GenerateKey(rand.Reader)
	s.updates.publicKey = filepath.Join(dir, "public.hex")
	s.updates.stagePath = filepath.Join(dir, "update", "staged.zip")
	s.updates.version = "0.7.0-stage5-a7"
	s.updates.platform = "linux/amd64"
	if err := os.WriteFile(s.updates.publicKey, []byte(hex.EncodeToString(pub)), 0644); err != nil {
		t.Fatal(err)
	}
	payload, err := updater.MakePackage([]byte(strings.Repeat("program bytes", 500)), "0.7.0-stage5-a8", key)
	if err != nil {
		t.Fatal(err)
	}
	release := updater.Release{Version: "0.7.0-stage5-a8", Platform: "linux/amd64", MinimumVersion: "0.7.0-stage5-a5", Scope: "core-ui", File: "update-a8.zip", Size: int64(len(payload))}
	hash := sha256.Sum256(payload)
	release.SHA256 = hex.EncodeToString(hash[:])
	raw, _ := json.Marshal(updater.Catalog{Format: "arcnet-update-catalog-v1", Releases: []updater.Release{release}})
	signed, err := updater.SignCatalog(raw, key)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := updater.VerifyCatalog(signed, pub)
	if err != nil {
		t.Fatal(err)
	}
	s.updates.catalog = func(context.Context, ed25519.PublicKey) (updater.Catalog, error) { return catalog, nil }

	downloading := make(chan struct{})
	proceed := make(chan struct{})
	var startedOnce sync.Once
	s.updates.download = func(ctx context.Context, _ updater.Release, _ ed25519.PublicKey) ([]byte, error) {
		startedOnce.Do(func() { close(downloading) })
		select {
		case <-proceed:
			return payload, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	socket := filepath.Join(dir, "control.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if p := s.updates.progress.Load(); p != nil && p.State == "downloading" {
			p.cancel()
		}
		listener.Close()
	})
	s.App.Core.ControlSocket = socket
	var mu sync.Mutex
	state := ops.UpdateStatus{Trusted: true, Result: "success", StartedAt: 10}
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			func() {
				defer conn.Close()
				b := make([]byte, 32)
				n, _ := conn.Read(b)
				mu.Lock()
				defer mu.Unlock()
				copy := state
				switch string(b[:n]) {
				case "update status\n":
					if blob, e := os.ReadFile(s.updates.stagePath); e == nil {
						if p, e := updater.Verify(blob, pub); e == nil {
							copy.Staged = true
							copy.Version = p.Manifest.Version
						}
					}
				case "update verify\n", "update apply\n":
					blob, e := os.ReadFile(s.updates.stagePath)
					if e != nil {
						return
					}
					p, e := updater.Verify(blob, pub)
					if e != nil {
						return
					}
					copy.Staged = true
					copy.Version = p.Manifest.Version
					if string(b[:n]) == "update apply\n" {
						copy.Accepted = true
						state.Busy = true
						state.StartedAt = 20
					}
				default:
					return
				}
				_ = json.NewEncoder(conn).Encode(copy)
			}()
		}
	}()
	return updateFixture{payload: payload, s: s, cookie: cookie, downloading: downloading, proceed: proceed, fail: func() { mu.Lock(); state.Busy = false; state.Result = "exit-code"; mu.Unlock() }}
}
func TestOnlineUpdateAuthStagingAndFailure(t *testing.T) {
	fixture := onlineUpdateFixture(t)
	s, cookie, downloading, proceed := fixture.s, fixture.cookie, fixture.downloading, fixture.proceed

	request := func(method, path, body string, auth, csrf bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "https://example.test"+path, strings.NewReader(body))
		if auth {
			r.AddCookie(cookie)
		}
		if csrf {
			r.Header.Set("X-CSRF-Token", "fixture-csrf")
		}
		w := httptest.NewRecorder()
		s.Routes().ServeHTTP(w, r)
		return w
	}
	if w := request("GET", "/api/system/update/releases", "", false, false); w.Code != 401 {
		t.Fatal("auth bypass", w.Code)
	}
	if w := request("POST", "/api/system/update/download", `{"version":"0.7.0-stage5-a8"}`, true, false); w.Code != 403 {
		t.Fatal("CSRF bypass", w.Code)
	}
	s.updates.catalogSlots <- struct{}{}
	s.updates.catalogSlots <- struct{}{}
	if w := request("GET", "/api/system/update/releases", "", true, false); w.Code != 429 {
		t.Fatal("unbounded catalog requests", w.Code)
	}
	<-s.updates.catalogSlots
	<-s.updates.catalogSlots
	if w := request("POST", "/api/system/update/download", `{"version":"0.7.0-stage5-a6"}`, true, true); w.Code != 409 {
		t.Fatal("downgrade accepted", w.Code)
	}
	if w := request("POST", "/api/system/update/download", `{"version":"0.7.0-stage5-a8"}`, true, true); w.Code != 202 {
		t.Fatal("start", w.Code, w.Body.String())
	}
	select {
	case <-downloading:
	case <-time.After(time.Second):
		t.Fatal("download not started")
	}
	if w := request("POST", "/api/system/update/discard", `{}`, true, true); w.Code != 409 {
		t.Fatal("concurrent stage mutation", w.Code)
	}
	close(proceed)
	deadline := time.Now().Add(2 * time.Second)
	for {
		p := s.updates.progress.Load()
		if p != nil && p.State != "downloading" {
			if p.State != "verified" {
				t.Fatal(p.Message)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("download timeout")
		}
		time.Sleep(5 * time.Millisecond)
	}
	r := httptest.NewRequest("POST", "https://example.test/api/system/update/upload", strings.NewReader("unsigned data"))
	r.AddCookie(cookie)
	r.Header.Set("X-CSRF-Token", "fixture-csrf")
	r.Header.Set("Content-Type", "application/octet-stream")
	bad := httptest.NewRecorder()
	s.Routes().ServeHTTP(bad, r)
	preserved, _ := os.ReadFile(s.updates.stagePath)
	if bad.Code != 422 || !bytes.Equal(preserved, fixture.payload) {
		t.Fatal("bad upload destroyed verified staged package")
	}
	if w := request("POST", "/api/system/update/apply", `{"confirm":"INSTALL_SIGNED_UPDATE","version":"0.7.0-stage5-a9"}`, true, true); w.Code != 409 {
		t.Fatal("staged version mismatch", w.Code)
	}
	if w := request("POST", "/api/system/update/apply", `{"confirm":"INSTALL_SIGNED_UPDATE","version":"0.7.0-stage5-a8"}`, true, true); w.Code != 202 {
		t.Fatal("apply", w.Code, w.Body.String())
	}
	if w := request("POST", "/api/system/update/discard", `{}`, true, true); w.Code != 409 {
		t.Fatal("discard while installing", w.Code)
	}
	fixture.fail()
	w := request("GET", "/api/system/update", "", true, false)
	if w.Code != 200 || s.updates.pending.Load() || !strings.Contains(w.Body.String(), `"current_version":"0.7.0-stage5-a7"`) {
		t.Fatal("failure not distinguished from success", w.Body.String())
	}
}

func TestOnlineUpdateBrowserFixture(t *testing.T) {
	if os.Getenv("ARCVPN_ONLINE_UPDATE_BROWSER") != "1" {
		t.Skip("opt-in browser fixture")
	}
	fixture := onlineUpdateFixture(t)
	s := fixture.s
	close(fixture.proceed)
	hash, err := auth.HashPassword("disposable-browser-password-12345")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.App.Store.CreateAdmin(context.Background(), "browser-owner", hash); err != nil {
		t.Fatal(err)
	}
	if err = s.WithWebRoot("../../frontend/dist"); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{}, 1)
	mux := http.NewServeMux()
	mux.Handle("/", s.Routes())
	mux.HandleFunc("POST /fixture-done", func(w http.ResponseWriter, r *http.Request) {
		select {
		case done <- struct{}{}:
		default:
		}
		w.WriteHeader(204)
	})
	server := &http.Server{Addr: "0.0.0.0:18394", Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	defer server.Close()
	result := make(chan error, 1)
	go func() { result <- server.ListenAndServe() }()
	t.Log("online update fixture ready :18394")
	select {
	case <-done:
	case err := <-result:
		t.Fatal(err)
	case <-time.After(3 * time.Minute):
		t.Fatal("browser fixture timeout")
	}
}

func TestOnlineUpdateCancellationLeavesNoPackage(t *testing.T) {
	f := onlineUpdateFixture(t)
	request := func(path string, csrf bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "https://example.test"+path, strings.NewReader(`{"version":"0.7.0-stage5-a8"}`))
		r.AddCookie(f.cookie)
		if csrf {
			r.Header.Set("X-CSRF-Token", "fixture-csrf")
		}
		w := httptest.NewRecorder()
		f.s.Routes().ServeHTTP(w, r)
		return w
	}
	if w := request("/api/system/update/download", true); w.Code != 202 {
		t.Fatal(w.Code, w.Body.String())
	}
	<-f.downloading
	if w := request("/api/system/update/download/cancel", false); w.Code != 403 {
		t.Fatal("cancel CSRF", w.Code)
	}
	if w := request("/api/system/update/download/cancel", true); w.Code != 202 {
		t.Fatal(w.Code)
	}
	deadline := time.Now().Add(time.Second)
	for len(f.s.updates.slots) > 0 {
		if time.Now().After(deadline) {
			t.Fatal("cancel did not release slot")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if _, err := os.Stat(f.s.updates.stagePath); !os.IsNotExist(err) {
		t.Fatal("cancelled download staged a package")
	}
	close(f.proceed)
	if w := request("/api/system/update/download", true); w.Code != 202 {
		t.Fatal("retry after cancellation", w.Code, w.Body.String())
	}
	deadline = time.Now().Add(time.Second)
	for len(f.s.updates.slots) > 0 {
		if time.Now().After(deadline) {
			t.Fatal("retry timeout")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if f.s.updates.progress.Load().State != "verified" {
		t.Fatal("retry did not verify")
	}
}
