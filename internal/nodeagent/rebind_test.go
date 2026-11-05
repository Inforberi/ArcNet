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

func TestAgentRebindIsAtomicAndDoesNotModifyLastGoodVPN(t *testing.T) {
	const token = "Aabcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHIJKL"
	const oldSecret = "Babcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHIJKL"
	const nextSecret = "Cabcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHIJKL"
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/nodes/register" || r.Method != "POST" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
			return
		}
		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload["token"] != token {
			t.Errorf("token not sent via JSON body")
			w.WriteHeader(403)
			return
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"node_id": "new-node", "secret": nextSecret})
	}))
	defer srv.Close()
	config := filepath.Join(t.TempDir(), "node.json")
	old := Configuration{Controller: "https://old.example", NodeID: "old-node", Secret: oldSecret}
	if err := saveConfiguration(config, old); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := rebind(context.Background(), srv.Client(), "https://bad@new.example", token, config); err == nil {
		t.Fatal("accepted credentials in controller origin")
	}
	after, err := os.ReadFile(config)
	if err != nil || string(after) != string(before) {
		t.Fatal("failed rebind modified the old identity")
	}
	if err := rebind(context.Background(), srv.Client(), srv.URL, token, config); err != nil {
		t.Fatal(err)
	}
	current, err := ReadConfig(config)
	if err != nil || current.Controller != srv.URL || current.NodeID != "new-node" || current.Secret != nextSecret || current.PreviousSecret != "" {
		t.Fatalf("new identity not atomically persisted: %+v %v", current, err)
	}
	st, err := os.Stat(config)
	if err != nil || st.Mode().Perm() != 0600 {
		t.Fatalf("unsafe permission: %v", err)
	}
	if err := rebind(context.Background(), srv.Client(), srv.URL, token, config); err == nil {
		t.Fatal("duplicate rebind to same controller")
	}
}
