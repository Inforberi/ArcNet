package app

import "errors"

// MutationError records a proven recovery result without exposing runtime output.
// Its cause is for internal error matching only, never for an HTTP response.
type MutationError struct {
	Cause     error
	Recovered bool
}

func (e *MutationError) Error() string { return e.Cause.Error() }
func (e *MutationError) Unwrap() error { return e.Cause }
func (a *App) mutationFailure(cause, recovery error) error {
	a.runtimeUncertain = recovery != nil
	if recovery != nil {
		a.usageApplied = ""
	}
	return &MutationError{Cause: errors.Join(cause, recovery), Recovered: recovery == nil}
}
