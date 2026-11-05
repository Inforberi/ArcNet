package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/arcvpn/arcvpn/internal/auth"
	"github.com/arcvpn/arcvpn/internal/engines"
	"github.com/arcvpn/arcvpn/internal/store"
	"github.com/arcvpn/arcvpn/internal/xray"
)

type App struct {
	Store           *store.Store
	Core            xray.Controller
	Engines         *engines.Manager
	ReservedTCPPort int // HTTPS administrative listener, never a VPN TCP listener
	mu              sync.Mutex
}

func (a *App) Setup(ctx context.Context, address, target string, port int) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	address = strings.TrimSpace(address)
	target = strings.TrimSpace(target)
	priv, pub, sid, err := xray.GenerateKeys(ctx, a.Core.Binary)
	if err != nil {
		return err
	}
	candidate := store.Reality{Address: address, Port: port, Target: target, SNI: target, Private: priv, Public: pub, ShortID: sid, Enabled: true}
	if err = xray.Validate(candidate); err != nil {
		return err
	}
	if candidate.Port == 80 || candidate.Port == a.ReservedTCPPort {
		return errors.New("VPN TCP port conflicts with HTTPS panel or ACME HTTP validation")
	}
	old, oldErr := a.Store.ReadReality(ctx)
	if oldErr == nil && old.Enabled {
		return errors.New("VPN already configured; changing keys requires a separate migration")
	}
	users, err := a.Store.ListUsers(ctx)
	if err != nil {
		return err
	}
	if err = a.applyAll(ctx, candidate, users); err != nil {
		return err
	}
	if err = a.Store.SaveReality(ctx, candidate); err != nil {
		if oldErr == nil {
			_ = a.applyAll(context.Background(), old, users)
		}
		return err
	}
	a.Store.Event(ctx, "vpn.setup", fmt.Sprintf("port %d", port))
	return nil
}
func (a *App) AddUser(ctx context.Context, name string) (store.User, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	name = strings.TrimSpace(name)
	if len([]rune(name)) < 1 || len([]rune(name)) > 60 || strings.ContainsAny(name, "\r\n\x00") {
		return store.User{}, errors.New("name must contain 1–60 characters")
	}
	r, err := a.Store.ReadReality(ctx)
	if err != nil {
		return store.User{}, errors.New("configure VPN first")
	}
	uuid, err := auth.UUID()
	if err != nil {
		return store.User{}, err
	}
	token, err := auth.Secret(32)
	if err != nil {
		return store.User{}, err
	}
	u := store.User{Name: name, UUID: uuid, Subscription: token, Enabled: true, CreatedAt: time.Now().Unix()}
	if err = a.Store.InsertUser(ctx, u); err != nil {
		return store.User{}, err
	}
	newUser, findErr := a.Store.ByToken(ctx, u.Subscription)
	if findErr != nil {
		return store.User{}, findErr
	}
	if err = a.ensureCredentials(ctx, newUser); err != nil {
		_ = a.Store.DeleteUser(context.Background(), newUser.ID)
		return store.User{}, err
	}
	users, err := a.Store.ListUsers(ctx)
	if err == nil {
		err = a.applyAll(ctx, r, users)
	}
	if err != nil {
		if rollbackUser, findErr := a.Store.ByToken(context.Background(), u.Subscription); findErr == nil {
			_ = a.Store.DeleteUser(context.Background(), rollbackUser.ID)
			if oldUsers, e := a.Store.ListUsers(context.Background()); e == nil {
				_ = a.applyAll(context.Background(), r, oldUsers)
			}
		}
		return store.User{}, fmt.Errorf("user not activated: %w", err)
	}
	for _, one := range users {
		if one.UUID == u.UUID {
			u.ID = one.ID
			break
		}
	}
	a.Store.Event(ctx, "user.created", fmt.Sprintf("ID %d", u.ID))
	return u, nil
}
func (a *App) ToggleUser(ctx context.Context, id int64, enabled bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	old, err := a.Store.ByID(ctx, id)
	if err != nil {
		return err
	}
	if old.Enabled == enabled {
		return nil
	}
	r, err := a.Store.ReadReality(ctx)
	if err != nil {
		return err
	}
	if err = a.Store.SetEnabled(ctx, id, enabled); err != nil {
		return err
	}
	users, err := a.Store.ListUsers(ctx)
	if err == nil {
		err = a.applyAll(ctx, r, users)
	}
	if err != nil {
		_ = a.Store.SetEnabled(context.Background(), id, old.Enabled)
		if oldUsers, e := a.Store.ListUsers(context.Background()); e == nil {
			_ = a.applyAll(context.Background(), r, oldUsers)
		}
		return fmt.Errorf("toggle not activated: %w", err)
	}
	a.Store.Event(ctx, "user.status", fmt.Sprintf("ID %d enabled=%t", id, enabled))
	return nil
}
func (a *App) DeleteUser(ctx context.Context, id int64) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	old, err := a.Store.ByID(ctx, id)
	if err != nil {
		return err
	}
	oldCredentials, err := a.Store.CredentialsForUser(ctx, id)
	if err != nil {
		return err
	}
	r, err := a.Store.ReadReality(ctx)
	if err != nil {
		return err
	}
	if err = a.Store.DeleteUser(ctx, id); err != nil {
		return err
	}
	users, err := a.Store.ListUsers(ctx)
	if err == nil {
		err = a.applyAll(ctx, r, users)
	}
	if err != nil {
		_ = a.Store.RestoreUser(context.Background(), old)
		_ = a.Store.SaveCredentials(context.Background(), oldCredentials)
		if oldUsers, e := a.Store.ListUsers(context.Background()); e == nil {
			_ = a.applyAll(context.Background(), r, oldUsers)
		}
		return fmt.Errorf("deletion not activated: %w", err)
	}
	a.Store.Event(ctx, "user.deleted", fmt.Sprintf("ID %d", id))
	return nil
}
