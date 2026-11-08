package app

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/arcvpn/arcvpn/internal/store"
)

func (a *App) EditUserPreferences(ctx context.Context, id int64, name string, p store.UserPreferences, expected int64) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	old, err := a.Store.ByID(ctx, id)
	if err != nil {
		return err
	}
	accessChanged := !reflect.DeepEqual(old.Preferences.AllowedKinds, p.AllowedKinds)
	if accessChanged && a.RuntimeCheck != nil {
		if err = a.RuntimeCheck(); err != nil {
			return err
		}
	}
	// Freeze legacy Amnezia paths until their identity model supports assignments.
	if accessChanged {
		profiles, e := a.Store.ListProtocols(ctx)
		if e != nil {
			return e
		}
		for _, v := range profiles {
			if v.Kind == "amneziawg" {
				return store.ErrUserAssignmentsUnavailable
			}
		}
	}
	if err = a.Store.SaveUserPreferences(ctx, id, name, p, expected); err != nil {
		return err
	}
	if accessChanged {
		reality, e := a.Store.OptionalReality(ctx)
		if e != nil {
			err = e
		} else {
			users, e := a.Store.ListUsers(ctx)
			if e != nil {
				err = e
			} else {
				err = a.applyAll(ctx, reality, users)
			}
		}
		if err != nil {
			undo, cancel := context.WithTimeout(context.Background(), 35*time.Second)
			defer cancel()
			restore := a.Store.SaveUserPreferences(undo, id, old.Name, old.Preferences, expected+1)
			if restore == nil {
				r, e := a.Store.OptionalReality(undo)
				restore = e
				if e == nil {
					users, e := a.Store.ListUsers(undo)
					restore = e
					if e == nil {
						restore = a.applyAll(undo, r, users)
					}
				}
			}
			return errors.Join(fmt.Errorf("user assignments apply failed: %w", err), restore)
		}
	}
	a.Store.Event(ctx, "user.preferences", fmt.Sprintf("ID %d", id))
	return nil
}
