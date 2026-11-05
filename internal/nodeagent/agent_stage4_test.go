package nodeagent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRotationRecoversLostConfirmationResponse(t *testing.T) {
	const oldSecret = "Aabcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHIJKL"
	const nextSecret = "Babcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHIJKL"
	current := oldSecret
	confirmed := false
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secret := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		switch r.URL.Path {
		case "/api/nodes/rotate/begin":
			if secret != oldSecret {
				t.Errorf("unexpected begin auth")
				w.WriteHeader(401)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"secret": nextSecret})
		case "/api/nodes/rotate/confirm":
			if !confirmed && secret == nextSecret {
				current = nextSecret
				confirmed = true
				w.WriteHeader(500)
				return
			}
			w.WriteHeader(401)
		case "/api/nodes/heartbeat":
			if secret != current {
				w.WriteHeader(401)
				return
			}
			w.WriteHeader(204)
		default:
			t.Errorf("unexpected endpoint %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	path := filepath.Join(t.TempDir(), "node.json")
	original := Configuration{Controller: srv.URL, NodeID: "node-1", Secret: oldSecret}
	if err := saveConfiguration(path, original); err != nil {
		t.Fatal(err)
	}
	if err := RotateCredential(context.Background(), srv.Client(), &original, path); err == nil {
		t.Fatal("lost confirmation not reported")
	}
	if original.PreviousSecret != oldSecret || original.Secret != nextSecret {
		t.Fatal("lost previous credential")
	}
	if err := RecoverCredential(context.Background(), srv.Client(), &original, path); err != nil {
		t.Fatal(err)
	}
	if original.Secret != nextSecret || original.PreviousSecret != "" {
		t.Fatal("committed new credential not retained")
	}
	fromDisk, err := ReadConfig(path)
	if err != nil || fromDisk.Secret != nextSecret || fromDisk.PreviousSecret != "" {
		t.Fatalf("recovery not durable: %v", err)
	}
	stat, err := os.Stat(path)
	if err != nil || stat.Mode().Perm() != 0600 {
		t.Fatal("insecure rotation file mode")
	}
}

func TestRotationRevertsExpiredPendingSecret(t *testing.T) {
	const oldSecret = "Aabcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHIJKL"
	const nextSecret = "Babcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHIJKL"
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secret := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		switch r.URL.Path {
		case "/api/nodes/rotate/confirm":
			w.WriteHeader(401)
		case "/api/nodes/heartbeat":
			if secret == oldSecret {
				w.WriteHeader(204)
			} else {
				w.WriteHeader(401)
			}
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	c := Configuration{Controller: srv.URL, NodeID: "node-1", Secret: nextSecret, PreviousSecret: oldSecret}
	path := filepath.Join(t.TempDir(), "node.json")
	if err := saveConfiguration(path, c); err != nil {
		t.Fatal(err)
	}
	if err := RecoverCredential(context.Background(), srv.Client(), &c, path); err != nil {
		t.Fatal(err)
	}
	if c.Secret != oldSecret || c.PreviousSecret != "" {
		t.Fatal("old credential not recovered")
	}
	disk, err := ReadConfig(path)
	if err != nil || disk.Secret != oldSecret {
		t.Fatal("fallback was not persisted")
	}
}
