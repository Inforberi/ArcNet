package httpapi

import (
	"net/http/httptest"
	"testing"

	"github.com/arcvpn/arcvpn/internal/store"
)

func TestCascadeBusyUsesConflictWithoutPrivateDetails(t *testing.T) {
	response := httptest.NewRecorder()
	handleErr(response, store.ErrCascadeBusy)
	if response.Code != 409 {
		t.Fatal("reservation conflict not surfaced", response.Code)
	}
}
