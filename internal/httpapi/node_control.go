package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/arcvpn/arcvpn/internal/store"
)

func (s *Server) nodeDeploy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")
	operation, e := s.App.DeployNode(ctx, id)
	if e != nil {
		switch {
		case errors.Is(e, store.ErrUsageUnsupported):
			handleErr(w, e)
		case errors.Is(e, store.ErrNotFound):
			fail(w, 404, "Нода не найдена")
		case errors.Is(e, store.ErrNodeBusy):
			fail(w, 409, "Предыдущее применение ещё выполняется")
		default:
			fail(w, 409, "Невозможно подготовить конфигурацию: "+safeNodeError(e))
		}
		return
	}
	s.App.Store.Event(ctx, "node.deploy", "queued deployment "+operation.ID)
	respond(w, 202, operation)
}
func safeNodeError(err error) string {
	// Never return an arbitrary error string from Xray / remote processes.
	switch err.Error() {
	case "node needs a verified public endpoint", "configure local VPN first", "node endpoint changed; identity migration required":
		return err.Error()
	}
	return "проверь настройки VPN, адрес ноды и доступность локального Xray"
}
func (s *Server) nodeDrain(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Draining *bool `json:"draining"`
	}
	if !decode(w, r, &request) {
		return
	}
	if request.Draining == nil {
		fail(w, http.StatusBadRequest, "Укажите draining: true или false")
		return
	}
	state, err := s.App.Store.SetNodeDraining(r.Context(), r.PathValue("id"), *request.Draining)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrNotFound):
			fail(w, http.StatusNotFound, "Нода не найдена")
		case errors.Is(err, store.ErrConflict):
			fail(w, http.StatusConflict, "Нельзя возобновить выдачу ссылок до подтверждения последней ревизии Xray")
		default:
			fail(w, http.StatusInternalServerError, "Не удалось изменить режим выдачи ссылок")
		}
		return
	}
	if state.Draining {
		s.App.Store.Event(r.Context(), "node.drain", "disabled new diagnostic links for node "+state.NodeID)
	} else {
		s.App.Store.Event(r.Context(), "node.resume", "resumed diagnostic links for node "+state.NodeID)
	}
	respond(w, http.StatusOK, state)
}

func (s *Server) nodeOperationList(w http.ResponseWriter, r *http.Request) {
	operations, e := s.App.Store.NodeOperations(r.Context(), r.PathValue("id"))
	if e != nil {
		fail(w, 500, "Не удалось получить операции")
		return
	}
	respond(w, 200, map[string]any{"items": operations})
}
func (s *Server) nodeRetry(w http.ResponseWriter, r *http.Request) {
	operation, e := s.App.Store.RetryNodeDeployment(r.Context(), r.PathValue("id"))
	if e != nil {
		if errors.Is(e, store.ErrNotFound) {
			fail(w, 404, "Нода не найдена")
		} else {
			fail(w, 409, "Повтор возможен только после ошибки применения")
		}
		return
	}
	s.App.Store.Event(r.Context(), "node.retry", "retry operation "+operation.ID)
	respond(w, 202, operation)
}
func (s *Server) nodeCancel(w http.ResponseWriter, r *http.Request) {
	e := s.App.Store.CancelNodeDeployment(r.Context(), r.PathValue("id"))
	if e != nil {
		fail(w, 409, "Можно отменить только ожидающую, но не выполняемую операцию")
		return
	}
	s.App.Store.Event(r.Context(), "node.cancel", "cancelled pending deployment")
	respond(w, 200, map[string]bool{"ok": true})
}
func agentBearer(r *http.Request) string {
	bearer := r.Header.Get("Authorization")
	if !strings.HasPrefix(bearer, "Bearer ") {
		return ""
	}
	return strings.TrimPrefix(bearer, "Bearer ")
}
func (s *Server) nodePoll(w http.ResponseWriter, r *http.Request) {
	nodeID := r.URL.Query().Get("node_id")
	token := agentBearer(r)
	if nodeID == "" || token == "" {
		http.Error(w, "Unauthorized", 401)
		return
	}
	if e := s.App.Store.MarkNodeStalled(r.Context(), 12*time.Minute); e != nil {
		http.Error(w, "Error", 500)
		return
	}
	if e := s.App.Store.AuthenticateNode(r.Context(), nodeID, token); e != nil {
		http.Error(w, "Unauthorized", 401)
		return
	}
	if e := s.App.ReconcileNodeCredentials(r.Context(), nodeID); e != nil && !errors.Is(e, store.ErrNodeBusy) {
		http.Error(w, "Remote credential policy is pending or unsupported", 409)
		return
	}
	command, e := s.App.Store.PendingNodeCommand(r.Context(), nodeID, token)
	if errors.Is(e, store.ErrNodeUnauthorized) || errors.Is(e, store.ErrNotFound) {
		http.Error(w, "Unauthorized", 401)
		return
	}
	if errors.Is(e, store.ErrIncompatibleNodeProtocol) {
		http.Error(w, "Agent protocol is incompatible or not reported", http.StatusConflict)
		return
	}
	if e != nil {
		http.Error(w, "Unable to retrieve pending operation", 500)
		return
	}
	if command == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	respond(w, 200, command)
}
func (s *Server) nodeAck(w http.ResponseWriter, r *http.Request) {
	token := agentBearer(r)
	if token == "" {
		http.Error(w, "Unauthorized", 401)
		return
	}
	var input struct {
		NodeID      string `json:"node_id"`
		OperationID string `json:"operation_id"`
		Revision    int64  `json:"revision"`
		SHA256      string `json:"sha256"`
		Result      string `json:"result"`
	}
	if !decode(w, r, &input) {
		return
	}
	err := s.App.Store.AcknowledgeNodeOperation(r.Context(), input.NodeID, token, input.OperationID, input.Revision, input.SHA256, input.Result)
	if errors.Is(err, store.ErrNodeUnauthorized) {
		http.Error(w, "Unauthorized", 401)
		return
	}
	if err != nil {
		http.Error(w, "Stale or malformed acknowledgment", 409)
		return
	}
	s.App.Store.Event(r.Context(), "node.ack", "operation "+input.OperationID+" status="+input.Result)
	w.WriteHeader(204)
}
func (s *Server) nodeRotateBegin(w http.ResponseWriter, r *http.Request) {
	token := agentBearer(r)
	if token == "" {
		http.Error(w, "Unauthorized", 401)
		return
	}
	var input struct {
		NodeID string `json:"node_id"`
	}
	if !decode(w, r, &input) {
		return
	}
	secret, err := s.App.Store.RotateNodeCredential(r.Context(), input.NodeID, token)
	if errors.Is(err, store.ErrNodeUnauthorized) {
		http.Error(w, "Unauthorized", 401)
		return
	}
	if err != nil {
		http.Error(w, "Rotation not available", 409)
		return
	}
	respond(w, 200, map[string]string{"secret": secret})
}
func (s *Server) nodeRotateConfirm(w http.ResponseWriter, r *http.Request) {
	token := agentBearer(r)
	if token == "" {
		http.Error(w, "Unauthorized", 401)
		return
	}
	var input struct {
		NodeID string `json:"node_id"`
	}
	if !decode(w, r, &input) {
		return
	}
	err := s.App.Store.ConfirmNodeCredential(r.Context(), input.NodeID, token)
	if err != nil {
		http.Error(w, "Invalid pending credential", 401)
		return
	}
	s.App.Store.Event(r.Context(), "node.rotate", "agent credentials rotated")
	w.WriteHeader(204)
}
