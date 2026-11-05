package httpapi

import (
	"context"
	"errors"
	"github.com/arcvpn/arcvpn/internal/nodebootstrap"
	"github.com/arcvpn/arcvpn/internal/store"
	"net/http"
	"os"
	"time"
)

func (s *Server) nodeSSHInspect(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Host string `json:"host"`
		Port int    `json:"port"`
	}
	if !decode(w, r, &input) {
		return
	}
	target, e := nodebootstrap.Inspect(r.Context(), input.Host, input.Port)
	if e != nil {
		fail(w, 400, "Не удалось безопасно определить SSH host key: "+nodebootstrap.Diagnostic(e))
		return
	}
	respond(w, 200, target)
}
func (s *Server) nodeSSHPreflight(w http.ResponseWriter, r *http.Request) {
	if s.publicURL == "" {
		fail(w, http.StatusConflict, "Публичный HTTPS URL контроллера не задан")
		return
	}
	var input nodebootstrap.Credentials
	if !decode(w, r, &input) {
		return
	}
	conn, e := nodebootstrap.Connect(r.Context(), input)
	if e != nil {
		fail(w, 400, "SSH проверка отклонена: "+nodebootstrap.Diagnostic(e))
		return
	}
	defer conn.Close()
	plan, e := conn.PreflightForController(r.Context(), s.publicURL)
	if e != nil {
		fail(w, 422, "SSH preflight не пройден: "+nodebootstrap.Diagnostic(e))
		return
	}
	respond(w, 200, plan)
}
func (s *Server) nodeSSHInstall(w http.ResponseWriter, r *http.Request) {
	if s.publicURL == "" {
		fail(w, 409, "Публичный HTTPS URL панели не настроен")
		return
	}
	var input struct {
		Name        string                    `json:"name"`
		Credentials nodebootstrap.Credentials `json:"credentials"`
		Approve     string                    `json:"approve"`
	}
	if !decode(w, r, &input) {
		return
	}
	if input.Approve != "CONFIRM_INSTALL" {
		fail(w, 400, "Для установки нужно явно подтвердить план изменений")
		return
	}
	if len([]rune(input.Name)) == 0 || len([]rune(input.Name)) > 64 {
		fail(w, 400, "Некорректное имя сервера")
		return
	}
	if e := input.Credentials.Validate(); e != nil {
		fail(w, 400, "Некорректные данные SSH")
		return
	}
	// Signing key and archive are operator-owned files. No fallback to a
	// controller-provided checksum and no install when missing.
	archive := os.Getenv("ARCVPN_NODE_RELEASE_BUNDLE")
	publicKey := os.Getenv("ARCVPN_NODE_RELEASE_KEY")
	release, e := nodebootstrap.LoadRelease(archive, publicKey)
	if e != nil {
		fail(w, 409, "Установка заблокирована: нужен проверенный подписанный пакет агента и Xray")
		return
	}
	// At most two concurrent SSH workers; do not allocate unbounded goroutines,
	// private keys or remote installation sessions under repeated admin clicks.
	select {
	case s.bootstrapSlots <- struct{}{}:
	default:
		fail(w, http.StatusTooManyRequests, "Уже выполняются две установки VPS; завершите одну из них")
		return
	}
	op, e := s.App.Store.CreateNodeBootstrap(r.Context(), input.Name, input.Credentials.Host, input.Credentials.SSHPort, input.Credentials.VPNPort)
	if e != nil {
		<-s.bootstrapSlots
		fail(w, 500, "Не удалось сохранить операцию установки")
		return
	}
	// Copy credentials only into this bounded worker. Browser closure does not
	// cancel it; controller restart marks the operation interrupted.
	go func() {
		defer func() { <-s.bootstrapSlots }()
		s.runNodeBootstrap(op.ID, input.Name, input.Credentials, release)
	}()
	respond(w, 202, op)
}
func (s *Server) runNodeBootstrap(id, name string, credentials nodebootstrap.Credentials, release nodebootstrap.Release) {
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	update := func(step, status, reason string) {
		// Persist the terminal state even if the SSH worker deadline has expired.
		writeCtx, writeCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer writeCancel()
		_ = s.App.Store.UpdateNodeBootstrap(writeCtx, id, step, status, reason)
	}
	step := "preflight"
	defer func() {
		if recovered := recover(); recovered != nil {
			update(step, "failed", "bootstrap worker crashed")
		}
	}()
	failStep := func(e error) {
		if e != nil {
			update(step, "failed", "SSH bootstrap failed at "+step)
			s.App.Store.Event(ctx, "node.bootstrap.failure", "bootstrap "+id+" step "+step)
		}
	}
	conn, e := nodebootstrap.Connect(ctx, credentials)
	if e != nil {
		failStep(e)
		return
	}
	defer conn.Close()
	preflight, e := conn.PreflightForController(ctx, s.publicURL)
	if e != nil {
		failStep(e)
		return
	}
	if len(preflight.Blockers) > 0 {
		failStep(errors.New("preflight blockers"))
		return
	}
	step = "verify_release"
	update(step, "running", "")
	step = "stage"
	update(step, "running", "")
	if e = conn.InstallVerified(ctx, release, id, func(next string) { step = next; update(next, "running", "") }); e != nil {
		failStep(e)
		return
	}
	step = "enroll"
	update(step, "running", "")
	// A failed enrollment must not silently leave a partially managed VPS.
	// If node.json is absent we can safely remove only this installation ID.
	// If the SSH reply was lost after registration, preserve the node and
	// explicitly demand manual recovery rather than deleting a live VPN.
	failEnrollment := func() {
		checkCtx, checkDone := context.WithTimeout(context.Background(), 20*time.Second)
		hasIdentity, checkErr := conn.RegistrationExists(checkCtx)
		checkDone()
		if checkErr != nil || hasIdentity {
			update(step, "failed", "registration state uncertain; inspect managed node on VPS")
			return
		}
		cleanupCtx, cleanupDone := context.WithTimeout(context.Background(), 90*time.Second)
		cleanupErr := conn.RemoveVerified(cleanupCtx, id, nodebootstrap.UninstallPurge)
		cleanupDone()
		if cleanupErr != nil {
			update(step, "failed", "enrollment failed; managed cleanup incomplete; inspect VPS")
			return
		}
		update(step, "failed", "enrollment failed; managed installation rolled back")
	}
	// Enrollment token is held only in memory, consumed by node-register using
	// SSH stdin and never embedded in subprocess arguments, logs, or SQLite.
	token, nodeID, e := s.App.Store.CreateEnrollmentForNodeWithID(ctx, name, conn.Target.ResolvedIP, credentials.VPNPort)
	if e != nil {
		failEnrollment()
		return
	}
	defer func() {
		revokeCtx, revokeCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer revokeCancel()
		_ = s.App.Store.NodeRevokeEnrollment(revokeCtx, token)
	}()
	if e = s.App.Store.AttachNodeBootstrap(ctx, id, nodeID); e != nil {
		failEnrollment()
		return
	}
	if e = conn.Register(ctx, s.publicURL, token); e != nil {
		failEnrollment()
		return
	}
	step = "apply"
	update(step, "running", "")
	deployment, deployErr := s.App.DeployNode(ctx, nodeID)
	if deployErr != nil {
		step = "manual_apply"
		update(step, "installed", "agent installed; configure local VPN and apply manually")
		s.App.Store.Event(ctx, "node.bootstrap", "agent installed; VPN config requires manual action")
		return
	}
	_ = deployment
	// The controller sends a pending revision. Completion requires an actual
	// agent acknowledgment and an active managed Xray service, not SSH alone.
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	probeTimeout := time.NewTimer(90 * time.Second)
	defer probeTimeout.Stop()
	for {
		select {
		case <-ctx.Done():
			update(step, "installed", "apply confirmation not received before timeout")
			return
		case <-probeTimeout.C:
			update(step, "installed", "apply confirmation pending; check the node status")
			return
		case <-ticker.C:
			control, stateErr := s.App.Store.NodeControl(ctx, nodeID)
			if stateErr != nil {
				continue
			}
			if control.DeploymentStatus == "failed" {
				update(step, "installed", "Xray apply failed; inspect the node deployment journal")
				return
			}
			if control.DeploymentStatus != "applied" {
				continue
			}
			nodes, nodeErr := s.App.Store.ListNodes(ctx)
			if nodeErr != nil {
				continue
			}
			for _, node := range nodes {
				if node.ID == nodeID && node.Online && node.XrayState == "active" {
					step = "finished"
					update(step, "installed", "")
					s.App.Store.Event(ctx, "node.bootstrap", "remote Xray applied; external VPN probe remains unknown")
					return
				}
			}
		}
	}
}
func (s *Server) nodeSSHOperations(w http.ResponseWriter, r *http.Request) {
	operations, e := s.App.Store.ListNodeBootstraps(r.Context())
	if e != nil {
		fail(w, 500, "Операции недоступны")
		return
	}
	respond(w, 200, map[string]any{"items": operations})
}
func (s *Server) nodeSSHOperation(w http.ResponseWriter, r *http.Request) {
	operation, e := s.App.Store.NodeBootstrap(r.Context(), r.PathValue("id"))
	if errors.Is(e, store.ErrNotFound) {
		fail(w, 404, "Операция не найдена")
		return
	}
	if e != nil {
		fail(w, 500, "Ошибка чтения операции")
		return
	}
	respond(w, 200, operation)
}
