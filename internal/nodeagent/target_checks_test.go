package nodeagent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/arcvpn/arcvpn/internal/store"
	"github.com/arcvpn/arcvpn/internal/targetcheck"
)

func TestTargetJobRejectsBadIdentityVersionExpiryAndDigestBeforeProbing(t *testing.T) {
	for _, kind := range []string{"old-controller", "none", "wrong-node", "version", "expired", "digest", "private-target"} {
		t.Run(kind, func(t *testing.T) {
			candidates := []targetcheck.Candidate{{Host: "example.com", SNI: "example.com"}}
			blob, _ := json.Marshal(candidates)
			digest := sha256.Sum256(blob)
			job := store.TargetJob{Version: 1, ID: "job", NodeID: "node-a", Candidates: candidates, Digest: hex.EncodeToString(digest[:]), ExpiresAt: time.Now().Add(time.Minute).Unix()}
			switch kind {
			case "wrong-node":
				job.NodeID = "node-b"
			case "version":
				job.Version = 9
			case "expired":
				job.ExpiresAt = 1
			case "digest":
				job.Digest = "wrong"
			case "private-target":
				job.Candidates[0].Host = "metadata.internal"
			}
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/nodes/target-checks/poll" || r.URL.Query().Get("node_id") != "node-a" || r.Header.Get("Authorization") != "Bearer test-secret" {
					t.Error("unexpected request or missing identity")
				}
				if kind == "old-controller" {
					w.WriteHeader(404)
					return
				}
				if kind == "none" {
					w.WriteHeader(204)
					return
				}
				_ = json.NewEncoder(w).Encode(job)
			}))
			defer srv.Close()
			err := PollTargetChecks(context.Background(), srv.Client(), Configuration{Controller: srv.URL, NodeID: "node-a", Secret: "test-secret"})
			if (kind == "old-controller" || kind == "none") != (err == nil) {
				t.Fatalf("%s: %v", kind, err)
			}
		})
	}
}
