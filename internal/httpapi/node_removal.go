package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/arcvpn/arcvpn/internal/nodebootstrap"
	"github.com/arcvpn/arcvpn/internal/store"
)

type nodeRemovalInput struct {
	Credentials    nodebootstrap.Credentials   `json:"credentials"`
	Mode           nodebootstrap.UninstallMode `json:"mode"`
	InstallationID string                      `json:"installation_id,omitempty"`
	Approve        string                      `json:"approve,omitempty"`
}

// nodeRemovalVerify requires both the original, persistent install journal and
// root-owned marker on the exact expected public IP. Admin-supplied SSH data
// alone cannot select an arbitrary VPS as a destructive target.
func (s *Server) nodeRemovalVerify(ctx context.Context, nodeID string, input nodeRemovalInput) (*nodebootstrap.Connection, nodebootstrap.RemovalPlan, error) {
	original, err := s.App.Store.InstalledBootstrapForNode(ctx, nodeID)
	if err != nil {
		return nil, nodebootstrap.RemovalPlan{}, err
	}
	control, err := s.App.Store.NodeControl(ctx, nodeID)
	if err != nil {
		return nil, nodebootstrap.RemovalPlan{}, err
	}
	plan, err := nodebootstrap.NewRemovalPlan(original.ID, input.Mode)
	if err != nil {
		return nil, nodebootstrap.RemovalPlan{}, err
	}
	if input.InstallationID != "" && input.InstallationID != original.ID {
		return nil, nodebootstrap.RemovalPlan{}, errors.New("installation identity changed")
	}
	if input.Credentials.VPNPort != control.Port || input.Credentials.SSHPort != original.SSHPort {
		return nil, nodebootstrap.RemovalPlan{}, errors.New("installation port mismatch")
	}
	conn, err := nodebootstrap.Connect(ctx, input.Credentials)
	if err != nil {
		return nil, nodebootstrap.RemovalPlan{}, err
	}
	if conn.Target.ResolvedIP != control.Address {
		conn.Close()
		return nil, nodebootstrap.RemovalPlan{}, errors.New("SSH target differs from managed node endpoint")
	}
	marker, err := conn.ReadManaged(ctx)
	if err != nil {
		conn.Close()
		return nil, nodebootstrap.RemovalPlan{}, err
	}
	if marker != original.ID && !(input.Mode == nodebootstrap.UninstallPurge && marker == "detached:"+original.ID) {
		conn.Close()
		return nil, nodebootstrap.RemovalPlan{}, errors.New("managed install marker mismatch")
	}
	if strings.HasPrefix(marker, "detached:") {
		plan.State = "detached"
	}
	return conn, plan, nil
}

func (s *Server) nodeSSHRemovePlan(w http.ResponseWriter, r *http.Request) {
	var input nodeRemovalInput
	if !decode(w, r, &input) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 40*time.Second)
	defer cancel()
	conn, plan, err := s.nodeRemovalVerify(ctx, r.PathValue("id"), input)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			fail(w, 404, "Для ноды нет подтверждённой SSH-установки")
		} else {
			fail(w, 409, "Не удалось подтвердить SSH, IP или managed marker; удаление запрещено")
		}
		return
	}
	conn.Close()
	respond(w, 200, plan)
}

func (s *Server) nodeSSHRemove(w http.ResponseWriter, r *http.Request) {
	var input nodeRemovalInput
	if !decode(w, r, &input) {
		return
	}
	expectedApproval := "CONFIRM_PURGE"
	if input.Mode == nodebootstrap.UninstallDetach {
		expectedApproval = "CONFIRM_DETACH"
	}
	if input.Approve != expectedApproval || input.InstallationID == "" {
		fail(w, 400, "Необходимо явно подтвердить режим удаления и ID исходной установки")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 40*time.Second)
	defer cancel()
	conn, plan, err := s.nodeRemovalVerify(ctx, r.PathValue("id"), input)
	if err != nil {
		fail(w, 409, "Удаление отклонено: неверная нода, SSH host key или managed marker")
		return
	}
	conn.Close()
	if input.InstallationID != plan.InstallationID {
		fail(w, 409, "Идентификатор установки изменился")
		return
	}
	select {
	case s.bootstrapSlots <- struct{}{}:
	default:
		fail(w, http.StatusTooManyRequests, "Другие операции SSH уже выполняются")
		return
	}
	op, err := s.App.Store.CreateNodeRemoval(ctx, r.PathValue("id"), string(input.Mode))
	if err != nil {
		<-s.bootstrapSlots
		if nodeRouteConflict(w, err) {
			return
		}
		fail(w, 409, "Нода уже занята или установка не подтверждена")
		return
	}
	go func() {
		defer func() { <-s.bootstrapSlots }()
		s.runNodeRemoval(op.ID, op.NodeID, plan.InstallationID, input)
	}()
	respond(w, http.StatusAccepted, op)
}

func (s *Server) runNodeRemoval(opID, nodeID, installID string, input nodeRemovalInput) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	update := func(step, status, reason string) {
		writeCtx, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		_ = s.App.Store.UpdateNodeBootstrap(writeCtx, opID, step, status, reason)
	}
	step := "inspect"
	defer func() {
		if recover() != nil {
			update(step, "failed", "SSH removal worker crashed")
		}
	}()
	update(step, "running", "")
	conn, err := nodebootstrap.Connect(ctx, input.Credentials)
	if err != nil {
		update(step, "failed", "SSH identity recheck failed")
		return
	}
	defer conn.Close()
	// Recheck IP + managed marker at execution time, not only during plan.
	control, err := s.App.Store.NodeControl(ctx, nodeID)
	if err != nil || conn.Target.ResolvedIP != control.Address {
		update(step, "failed", "SSH destination changed before removal")
		return
	}
	step = "remove"
	update(step, "running", "")
	if err = conn.RemoveVerified(ctx, installID, input.Mode); err != nil {
		update(step, "failed", "Managed resource removal did not complete; inspect VPS console")
		return
	}
	step = "revoke"
	update(step, "running", "")
	if err = s.App.Store.RemoveNode(ctx, nodeID); err != nil && !errors.Is(err, store.ErrNotFound) {
		update(step, "failed", "Remote resources removed; controller identity requires manual revocation")
		return
	}
	s.App.Store.Event(ctx, "node.remove.ssh", "managed node "+nodeID+" "+string(input.Mode))
	if input.Mode == nodebootstrap.UninstallDetach {
		update("finished", "detached", "")
	} else {
		update("finished", "removed", "")
	}
}
