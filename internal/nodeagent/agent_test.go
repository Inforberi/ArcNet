package nodeagent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestRegistrationUsesTLSAndWritesRestrictedCredentials(t *testing.T) {
	const token = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ012345678901"
	var registered, heartbeatCalled bool
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/nodes/register":
			if r.Method != "POST" {
				t.Error("invalid registration method")
			}
			var in struct {
				Token string `json:"token"`
			}
			if e := json.NewDecoder(r.Body).Decode(&in); e != nil || in.Token != token {
				t.Errorf("registration payload: %+v %v", in, e)
			}
			registered = true
			w.WriteHeader(201)
			_ = json.NewEncoder(w).Encode(map[string]string{"node_id": "uuid", "secret": "Abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklmno"})
		case "/api/nodes/heartbeat":
			if r.Header.Get("Authorization") != "Bearer Abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklmno" {
				t.Error("missing bearer authorization")
			}
			heartbeatCalled = true
			w.WriteHeader(204)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	path := filepath.Join(t.TempDir(), "node.json")
	ctx := context.Background()
	if err := register(ctx, srv.Client(), srv.URL, token, path); err != nil {
		t.Fatal(err)
	}
	if !registered {
		t.Fatal("registration not sent")
	}
	if err := register(ctx, srv.Client(), srv.URL, token, path); err == nil {
		t.Fatal("overwritten node identity")
	}
	stat, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if stat.Mode().Perm() != 0600 {
		t.Fatalf("node credential mode: %o", stat.Mode().Perm())
	}
	c, err := ReadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Controller != srv.URL || c.NodeID != "uuid" {
		t.Fatalf("bad agent config: %+v", c)
	}
	if err := heartbeat(ctx, srv.Client(), c); err != nil {
		t.Fatal(err)
	}
	if !heartbeatCalled {
		t.Fatal("heartbeat not sent")
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadConfig(path); err == nil {
		t.Fatal("insecure credentials file accepted")
	}
}
func TestControllerRejectsNonHTTPSAndRedirects(t *testing.T) {
	for _, v := range []string{"http://example.com", "https://example.com/path", "https://a:b@example.com", "https://example.com?a=b", "not a URL"} {
		if _, err := ValidateOrigin(v); err == nil {
			t.Fatalf("accepted %q", v)
		}
	}
	if Client().CheckRedirect == nil {
		t.Fatal("default redirects may leak credentials")
	}
}
