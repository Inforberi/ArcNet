package xray

// OperationError identifies a safe failure stage without exposing Xray output.
type OperationError struct {
	Stage string
	Err   error
}

func (e *OperationError) Error() string { return e.Err.Error() }
func (e *OperationError) Unwrap() error { return e.Err }
