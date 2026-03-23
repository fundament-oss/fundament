package errors

import "errors"

// Transient wraps an error to indicate it is temporary and can be retried
// with exponential backoff. The plugin status is set to "degraded".
type Transient struct {
	err error
}

// NewTransient wraps err as a transient (retryable) error.
func NewTransient(err error) error {
	return &Transient{err: err}
}

func (e *Transient) Error() string {
	return e.err.Error()
}

func (e *Transient) Unwrap() error {
	return e.err
}

// IsTransient reports whether err or any error in its chain is transient.
func IsTransient(err error) bool {
	var t *Transient
	return errors.As(err, &t)
}

// Permanent wraps an error to indicate it is not retryable and requires
// human intervention. The plugin status is set to "failed".
type Permanent struct {
	err error
}

// NewPermanent wraps err as a permanent (non-retryable) error.
func NewPermanent(err error) error {
	return &Permanent{err: err}
}

func (e *Permanent) Error() string {
	return e.err.Error()
}

func (e *Permanent) Unwrap() error {
	return e.err
}

// IsPermanent reports whether err or any error in its chain is permanent.
func IsPermanent(err error) bool {
	var p *Permanent
	return errors.As(err, &p)
}
