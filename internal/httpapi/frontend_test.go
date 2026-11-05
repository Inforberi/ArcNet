package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arcvpn/arcvpn/internal/app"
	"github.com/arcvpn/arcvpn/internal/db"
	"github.com/arcvpn/arcvpn/internal/store"
)

// These tests exercise the exact static-asset and client-route contract used by
// the installer without pretending that a hand-written HTML fixture is a React build.
func TestProductionFrontendRoutesAndNodeInstaller(t *testing.T) {
	dataDir := t.TempDir()
	sqlite, err := db.Open(filepath.Join(dataDir, "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer sqlite.Close()
	st := &store.Store{DB: sqlite}
	if err := st.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	dist := filepath.Join(dataDir, "dist")
	if err := os.MkdirAll(filepath.Join(dist, "assets"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dist, "index.html"), []byte(`<!doctype html><html><head><title>ArcVPN test bundle</title><script type="module" crossorigin src="/assets/index-123.js"></script><link rel="stylesheet" href="/assets/index-123.css"></head><body><div id="root"></div></body></html>`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dist, "assets", "index-123.js"), []byte("export const productionBundle = true;"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dist, "assets", "index-123.css"), []byte("body{color:white}"), 0600); err != nil {
		t.Fatal(err)
	}
	s := New(&app.App{Store: st}, slog.Default())
	if err := s.WithWebRoot(dist); err != nil {
		t.Fatal(err)
	}
	handler := s.Routes()
	for _, tc := range []struct {
		path     string
		status   int
		expected string
	}{
		{"/", 200, "ArcVPN test bundle"},
		{"/users", 200, "ArcVPN test bundle"},
		{"/users/123", 200, "ArcVPN test bundle"},
		{"/routing", 200, "ArcVPN test bundle"},
		{"/assets/index-123.js", 200, "productionBundle"},
		{"/node-install.sh", 200, "#!/usr/bin/env bash"},
		{"/api/not-a-real-endpoint", 404, "404 page not found"},
		{"/assets/missing.js", 404, "404 page not found"},
	} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://127.0.0.1"+tc.path, nil))
		if rec.Code != tc.status || !strings.Contains(rec.Body.String(), tc.expected) {
			t.Fatalf("%s: status=%d body=%q", tc.path, rec.Code, rec.Body.String())
		}
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://127.0.0.1/node-install.sh", nil))
	if rec.Header().Get("Content-Type") != "text/x-shellscript; charset=utf-8" {
		t.Fatal("incorrect node installer MIME type")
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("node installer should not be cached")
	}
	if _, err := io.ReadAll(rec.Body); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/", "/users", "/nodes"} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://127.0.0.1"+path, nil))
		if rec.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("SPA HTML %s must not be cached", path)
		}
	}
}

// A Vite index is not enough: every referenced entrypoint must exist and no
// symlink may expose files outside the static root.
func TestReactWebRootRejectsIncompleteAssets(t *testing.T) {
	for _, tc := range []struct {
		name       string
		index      string
		addJS      bool
		addCSS     bool
		addSymlink bool
	}{
		{"missing root", `<html><script src="/assets/index-a.js"></script><link href="/assets/index-a.css" rel="stylesheet"></html>`, true, true, false},
		{"missing JavaScript", `<div id="root"></div><link href="/assets/index-a.css" rel="stylesheet">`, true, true, false},
		{"missing CSS", `<div id="root"></div><script type="module" src="/assets/index-a.js"></script>`, true, false, false},
		{"missing referenced JavaScript", `<div id="root"></div><script type="module" src="/assets/index-a.js"></script><link href="/assets/index-a.css" rel="stylesheet">`, false, true, false},
		{"symlink in bundle", `<div id="root"></div><script type="module" src="/assets/index-a.js"></script><link href="/assets/index-a.css" rel="stylesheet">`, true, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.MkdirAll(filepath.Join(dir, "assets"), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(tc.index), 0600); err != nil {
				t.Fatal(err)
			}
			if tc.addJS {
				if err := os.WriteFile(filepath.Join(dir, "assets", "index-a.js"), []byte("export {}"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if tc.addCSS {
				if err := os.WriteFile(filepath.Join(dir, "assets", "index-a.css"), []byte("body{}"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if tc.addSymlink {
				if err := os.Symlink("/etc/passwd", filepath.Join(dir, "assets", "external")); err != nil {
					t.Fatal(err)
				}
			}
			if err := New(&app.App{}, slog.Default()).WithWebRoot(dir); err == nil {
				t.Fatal("expected invalid frontend bundle to be rejected")
			}
		})
	}
}
