package httpapi

import (
	"context"
	"errors"
	"net/http"
	"os"
	"time"

	"github.com/arcvpn/arcvpn/internal/nodebootstrap"
	"github.com/arcvpn/arcvpn/internal/store"
)

type nodeUpgradeInput struct {
	Credentials     nodebootstrap.Credentials `json:"credentials"`
	InstallationID  string                    `json:"installation_id,omitempty"`
	ExpectedCurrent string                    `json:"expected_current,omitempty"`
	Approve         string                    `json:"approve,omitempty"`
}

type nodeUpgradePlan struct {
	InstallationID string   `json:"installation_id"`
	CurrentVersion string   `json:"current_version"`
	TargetVersion  string   `json:"target_version"`
	Changes        []string `json:"changes"`
}

func (s *Server) verifyNodeUpgrade(ctx context.Context, nodeID string, input nodeUpgradeInput, targetVersion string) (nodeUpgradePlan, error) {
	original, err := s.App.Store.InstalledBootstrapForNode(ctx, nodeID)
	if err != nil {
		return nodeUpgradePlan{}, err
	}
	control, err := s.App.Store.NodeControl(ctx, nodeID)
	if err != nil {
		return nodeUpgradePlan{}, err
	}
	if input.InstallationID != "" && input.InstallationID != original.ID {
		return nodeUpgradePlan{}, errors.New("installation identity differs")
	}
	if input.Credentials.SSHPort != original.SSHPort || input.Credentials.VPNPort != control.Port {
		return nodeUpgradePlan{}, errors.New("managed ports differ")
	}
	conn, err := nodebootstrap.Connect(ctx, input.Credentials)
	if err != nil {
		return nodeUpgradePlan{}, err
	}
	defer conn.Close()
	if conn.Target.ResolvedIP != control.Address {
		return nodeUpgradePlan{}, errors.New("SSH destination differs from managed endpoint")
	}
	marker, err := conn.ReadManaged(ctx)
	if err != nil {
		return nodeUpgradePlan{}, err
	}
	if marker != original.ID {
		return nodeUpgradePlan{}, errors.New("detached or foreign installation cannot be upgraded")
	}
	current, err := conn.ReadVersion(ctx)
	if err != nil {
		return nodeUpgradePlan{}, err
	}
	if err = nodebootstrap.UpgradeAvailable(current, targetVersion); err != nil {
		return nodeUpgradePlan{}, err
	}
	if input.ExpectedCurrent != "" && input.ExpectedCurrent != current {
		return nodeUpgradePlan{}, errors.New("node release changed since approval")
	}
	return nodeUpgradePlan{InstallationID: original.ID, CurrentVersion: current, TargetVersion: targetVersion,
		Changes: []string{"Verify controller-owned signed agent and Xray release", "Copy verified binaries to pinned VPS", "Test candidate Xray against current configuration", "Stop only ArcVPN services, replace both binaries and restart", "On service error restore previous agent and Xray binaries", "Preserve current VPN config, SSH and firewall; external handshake still requires testing"}}, nil
}

func loadNodeUpgradeRelease() (nodebootstrap.Release, error) {
	return nodebootstrap.LoadRelease(os.Getenv("ARCVPN_NODE_RELEASE_BUNDLE"), os.Getenv("ARCVPN_NODE_RELEASE_KEY"))
}

func (s *Server) nodeSSHUpgradePlan(w http.ResponseWriter, r *http.Request) {
	var input nodeUpgradeInput
	if !decode(w, r, &input) {
		return
	}
	release, err := loadNodeUpgradeRelease()
	if err != nil {
		fail(w, http.StatusConflict, "Обновление невозможно: отсутствует корректный подписанный пакет релиза")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	plan, err := s.verifyNodeUpgrade(ctx, r.PathValue("id"), input, release.Version)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			fail(w, 404, "Отсутствует исходная SSH-установка")
		} else {
			fail(w, 409, "Проверка версии, идентификатора ноды и SSH не пройдена")
		}
		return
	}
	respond(w, 200, plan)
}

func (s *Server) nodeSSHUpgrade(w http.ResponseWriter, r *http.Request) {
	var input nodeUpgradeInput
	if !decode(w, r, &input) {
		return
	}
	if input.Approve != "CONFIRM_UPGRADE" || input.InstallationID == "" || input.ExpectedCurrent == "" {
		fail(w, 400, "Требуется точное подтверждение обновления, ID установки и текущей версии")
		return
	}
	release, err := loadNodeUpgradeRelease()
	if err != nil {
		fail(w, 409, "Корректный подписанный пакет обновления отсутствует")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	plan, err := s.verifyNodeUpgrade(ctx, r.PathValue("id"), input, release.Version)
	if err != nil {
		fail(w, 409, "Версия или SSH-идентификатор изменились; повторите проверку")
		return
	}
	if input.InstallationID != plan.InstallationID || input.ExpectedCurrent != plan.CurrentVersion {
		fail(w, 409, "Операция отличается от подтверждённого плана")
		return
	}
	select {
	case s.bootstrapSlots <- struct{}{}:
	default:
		fail(w, 429, "Две SSH-операции уже выполняются")
		return
	}
	op, err := s.App.Store.CreateNodeUpgrade(ctx, r.PathValue("id"), release.Version)
	if err != nil {
		<-s.bootstrapSlots
		fail(w, 409, "Нода занята другой SSH-операцией")
		return
	}
	go func() {
		defer func() { <-s.bootstrapSlots }()
		s.runNodeUpgrade(op.ID, op.NodeID, plan.InstallationID, input, release)
	}()
	respond(w, 202, op)
}

func (s *Server) runNodeUpgrade(opID, nodeID, installID string, input nodeUpgradeInput, release nodebootstrap.Release) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	update := func(step, status, reason string) {
		c, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		_ = s.App.Store.UpdateNodeBootstrap(c, opID, step, status, reason)
	}
	step := "inspect"
	defer func() {
		if recover() != nil {
			update(step, "failed", "SSH upgrade worker crashed")
		}
	}()
	update(step, "running", "")
	plan, err := s.verifyNodeUpgrade(ctx, nodeID, input, release.Version)
	if err != nil || plan.InstallationID != installID || plan.CurrentVersion != input.ExpectedCurrent {
		update(step, "failed", "SSH identity or release changed after approval")
		return
	}
	conn, err := nodebootstrap.Connect(ctx, input.Credentials)
	if err != nil {
		update(step, "failed", "Pinned SSH connection failed")
		return
	}
	defer conn.Close()
	step = "upgrade"
	update(step, "running", "")
	if err = conn.UpgradeVerified(ctx, release, installID, opID); err != nil {
		update(step, "failed", "Signed upgrade failed; verify managed services and rollback on VPS")
		return
	}
	update("finished", "upgraded", "")
	s.App.Store.Event(ctx, "node.upgrade.ssh", "signed release "+release.Version+" applied to node "+nodeID+"; external VPN check outstanding")
}
