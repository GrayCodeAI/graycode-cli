package hawkerr

import "fmt"

// BridgeError represents a structured error from a bridge component.
// It captures which bridge failed, the operation attempted, and the
// underlying reason or wrapped error.
type BridgeError struct {
	Bridge string // e.g., "harrier", "swift", "kestrel", "merlin"
	Op     string // e.g., "Remember", "Recall", "Enable"
	Reason string // human-readable reason
	Err    error  // underlying error, if any
}

func (e *BridgeError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s bridge: %s: %s: %v", e.Bridge, e.Op, e.Reason, e.Err)
	}
	return fmt.Sprintf("%s bridge: %s: %s", e.Bridge, e.Op, e.Reason)
}

func (e *BridgeError) Unwrap() error {
	return e.Err
}

// NewBridgeError creates a BridgeError with optional wrapped error.
func NewBridgeError(bridge, op, reason string, err error) *BridgeError {
	return &BridgeError{
		Bridge: bridge,
		Op:     op,
		Reason: reason,
		Err:    err,
	}
}
