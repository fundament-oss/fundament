package logs

import "fmt"

// StatusError reports a non-2xx response from the log backend, so callers can
// react to specific statuses (credential rotation on 401, datasource-id drift
// on 404/500 behind the Plutono proxy) instead of string-matching.
type StatusError struct {
	StatusCode int
	Operation  string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("%s: status %d", e.Operation, e.StatusCode)
}
