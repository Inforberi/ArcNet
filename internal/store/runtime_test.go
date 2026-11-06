package store

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestRuntimePlanEnabledFlagsWithoutSecrets(t *testing.T) {
	st, done := createControlTestStore(t)
	defer done()
	ctx := context.Background()
	if err := st.SaveReality(ctx, Reality{Private: "secret-do-not-export", Enabled: false}); err != nil {
		t.Fatal(err)
	}
	for _, p := range []Protocol{{Kind: "vless-xhttp", Enabled: true}, {Kind: "hysteria2", Enabled: false}, {Kind: "wireguard", Enabled: true}, {Kind: "amneziawg", Enabled: false}} {
		if err := st.UpsertProtocol(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	plan, err := st.RuntimePlan(ctx)
	if err != nil || !plan.Xray || !plan.WireGuard || plan.Hysteria || plan.LegacyAmnezia {
		t.Fatal(plan, err)
	}
	encoded, _ := json.Marshal(plan)
	if strings.Contains(string(encoded), "secret") {
		t.Fatal("runtime DTO leaked secrets")
	}
}
