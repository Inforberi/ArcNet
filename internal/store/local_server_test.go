package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/arcvpn/arcvpn/internal/usage"
)

func TestIndependentServerAddressAndAccountingSources(t *testing.T) {
	for _, tc := range []struct{ kind, source, stream string }{{"hysteria2", "hysteria", "hysteria2"}, {"wireguard", "wireguard", "wireguard"}} {
		t.Run(tc.kind, func(t *testing.T) {
			st, u := usageFixture(t)
			ctx := context.Background()
			if _, err := st.LocalServer(ctx); !errors.Is(err, ErrNotFound) {
				t.Fatal(err)
			}
			if err := st.UpsertProtocol(ctx, Protocol{Kind: tc.kind, Address: "198.51.100.25", Enabled: true}); err != nil {
				t.Fatal(err)
			}
			server, err := st.LocalServer(ctx)
			if err != nil || server.Address != "198.51.100.25" {
				t.Fatal(server, err)
			}
			if fresh, _, _ := st.AccountingFresh(ctx); fresh {
				t.Fatal("unobserved source fresh")
			}
			now := time.Now().Unix()
			counter := usage.Counter{UserID: u.ID, Identity: u.UUID, Stream: tc.stream, Epoch: "first", StartedAt: 1, ObservedAt: now, Upload: 100}
			if err := st.RecordEngineUsage(ctx, tc.source, []usage.Counter{counter}, now, ""); err != nil {
				t.Fatal(err)
			}
			if fresh, _, err := st.AccountingFresh(ctx); err != nil || !fresh {
				t.Fatal("unconfigured Xray blocks supported source", err)
			}
			if err := st.SaveReality(ctx, Reality{Address: server.Address, Enabled: true}); err != nil {
				t.Fatal(err)
			}
			if fresh, _, _ := st.AccountingFresh(ctx); fresh {
				t.Fatal("new unobserved Xray ignored")
			}
			if err := st.UpsertProtocol(ctx, Protocol{Kind: "vless-xhttp", Address: "198.51.100.26"}); err != nil {
				t.Fatal(err)
			}
			if _, err := st.LocalServer(ctx); err == nil {
				t.Fatal("conflicting addresses accepted")
			}
		})
	}
}

func TestLocalServerPreservesExistingIPv6Representation(t *testing.T) {
	st, done := createControlTestStore(t)
	defer done()
	ctx := context.Background()
	address := "2001:db8:0:0::ABCD"
	if err := st.SaveReality(ctx, Reality{Address: address}); err != nil {
		t.Fatal(err)
	}
	server, err := st.LocalServer(ctx)
	if err != nil || server.Address != address {
		t.Fatal("legacy address changed", server, err)
	}
}
