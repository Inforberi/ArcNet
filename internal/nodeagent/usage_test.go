package nodeagent

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/arcvpn/arcvpn/internal/noderuntime"
	"github.com/arcvpn/arcvpn/internal/usage"
)

func TestAgentUsageBindsNodeAndBearerAndHandlesOldHelper(t *testing.T) {
	for _, tc := range []struct {
		name       string
		capability int
		response   int
		invalid    bool
		wantError  bool
	}{
		{"accepted", 1, 204, false, false},
		{"old-helper", 0, 204, false, false},
		{"invalid-root-reply", 1, 204, true, true},
		{"controller-refused", 1, 401, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const secret = "private-node-bearer-for-test-only"
			const node = "11111111-1111-4111-8111-111111111111"
			calls := make(chan bool, 1)
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var report usage.NodeReport
				ok := json.NewDecoder(r.Body).Decode(&report) == nil && report.NodeID == node && r.Header.Get("Authorization") == "Bearer "+secret && r.URL.Path == "/api/nodes/usage" && r.Method == http.MethodPost
				calls <- ok
				w.WriteHeader(tc.response)
			}))
			defer srv.Close()
			socket := filepath.Join(t.TempDir(), "helper.sock")
			listener, e := net.Listen("unix", socket)
			if e != nil {
				t.Fatal(e)
			}
			defer listener.Close()
			done := make(chan error, 1)
			go func() {
				requests := 1
				if tc.capability == 1 {
					requests = 2
				}
				for i := 0; i < requests; i++ {
					conn, e := listener.Accept()
					if e != nil {
						done <- e
						return
					}
					var req noderuntime.ApplyRequest
					e = json.NewDecoder(conn).Decode(&req)
					if e == nil {
						e = noderuntime.ValidateRequest(req)
					}
					if e != nil {
						conn.Close()
						done <- e
						return
					}
					reply := noderuntime.ApplyResponse{Status: "capabilities", CascadeProtocol: 1, UsageProtocol: tc.capability}
					if i == 1 {
						reply = noderuntime.ApplyResponse{Status: "statistics", Usage: &usage.NodeReport{Version: 1, NodeID: "untrusted-helper-node", Epoch: "epoch", StartedAt: 1, ObservedAt: 1}}
						if tc.invalid {
							reply.Usage = nil
						}
					}
					e = json.NewEncoder(conn).Encode(reply)
					conn.Close()
					if e != nil {
						done <- e
						return
					}
				}
				done <- nil
			}()
			e = reportNodeUsage(context.Background(), srv.Client(), Configuration{Controller: srv.URL, NodeID: node, Secret: secret}, socket)
			if (e != nil) != tc.wantError {
				t.Fatal("result", e)
			}
			if e = <-done; e != nil {
				t.Fatal(e)
			}
			if tc.capability == 1 && !tc.invalid {
				if ok := <-calls; !ok {
					t.Fatal("unscoped report")
				}
			} else {
				select {
				case <-calls:
					t.Fatal("unexpected HTTP report")
				default:
				}
			}
		})
	}
}
