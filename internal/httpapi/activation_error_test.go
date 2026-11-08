package httpapi

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/arcvpn/arcvpn/internal/engines"
	"github.com/arcvpn/arcvpn/internal/xray"
)

func TestActivationEngineFailureIsSafe(t *testing.T) {
	for _, stage := range []string{"runtime", "configuration", "validation", "status", "restart", "rollback"} {
		t.Run(stage, func(t *testing.T) {
			w := httptest.NewRecorder()
			activationFailure(w, &engines.OperationError{Kind: "wireguard", Stage: stage, Err: errors.New("private-key-secret")})
			if w.Code != 422 || !strings.Contains(w.Body.String(), "engine_wireguard_"+stage) || strings.Contains(w.Body.String(), "private-key-secret") {
				t.Fatal(w.Body.String())
			}
		})
	}
}

func TestActivationOuterXrayRollbackWins(t *testing.T) {
	w := httptest.NewRecorder()
	activationFailure(w, &xray.OperationError{Stage: "rollback", Err: &engines.OperationError{Kind: "wireguard", Stage: "restart"}})
	if !strings.Contains(w.Body.String(), "xray_rollback") {
		t.Fatal(w.Body.String())
	}
}
