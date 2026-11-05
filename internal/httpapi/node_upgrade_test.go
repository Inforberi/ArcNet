package httpapi

import (
	"context"
	"log/slog"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arcvpn/arcvpn/internal/app"
	"github.com/arcvpn/arcvpn/internal/db"
	"github.com/arcvpn/arcvpn/internal/store"
)

func TestStage4UpgradeEndpointsRejectAnonymousAndUnconfirmed(t *testing.T) {
	ctx := context.Background()
	database, err := db.Open(filepath.Join(t.TempDir(), "api.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	state := &store.Store{DB: database}
	if err = state.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	handler := New(&app.App{Store: state}, slog.Default()).Routes()
	for _, path := range []string{"/api/nodes/unauthorized/ssh/upgrade-plan", "/api/nodes/unauthorized/ssh/upgrade", "/api/nodes/unauthorized/ssh/remove"} {
		r := httptest.NewRequest("POST", path, strings.NewReader(`{"approve":"CONFIRM_UPGRADE"}`))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != 401 {
			t.Errorf("%s not protected: %d", path, w.Code)
		}
	}
}
