package trip

import "errors"

var (
	ErrDriverBusy        = errors.New("driver already has an active trip")
	ErrTripNotFound      = errors.New("trip not found")
	ErrTripCompleted     = errors.New("trip is already completed")
	ErrFinishBeforeStart = errors.New("trip cannot be finished before it was started")
)

type ValidationError struct {
	Field  string
	Reason string
}

func (e *ValidationError) Error() string {
	return e.Field + ": " + e.Reason
}
