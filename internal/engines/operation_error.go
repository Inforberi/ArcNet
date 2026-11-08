package engines

// OperationError identifies the failed step without exposing validator output or keys.
type OperationError struct {
	Kind, Stage string
	Err         error
}

func (e *OperationError) Error() string { return e.Kind + " engine " + e.Stage + " failed" }
func (e *OperationError) Unwrap() error { return e.Err }
