package provider

import (
	"time"

	"connectrpc.com/connect"
)

const retryAttempts = 5

// retryOnPermissionDenied calls fn up to retryAttempts times.
// Between attempts it sleeps attempt*second (linear back-off).
// It retries only on CodePermissionDenied; any other error – or a
// permission-denied on the final attempt – is returned immediately.
func retryOnPermissionDenied[T any](fn func() (*connect.Response[T], error)) (*connect.Response[T], error) {
	var (
		result *connect.Response[T]
		err    error
	)
	for attempt := range retryAttempts {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * time.Second)
		}

		result, err = fn()
		if err == nil {
			return result, nil
		}

		if connect.CodeOf(err) != connect.CodePermissionDenied || attempt == retryAttempts-1 {
			return nil, err
		}
	}

	return nil, err
}
