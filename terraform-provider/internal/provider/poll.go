package provider

import (
	"context"
	"time"
)

// pollWithBackoff repeatedly invokes poll every interval until it returns
// done==true, a fatal error, or ctx is cancelled.
//
// The poll function classifies its own outcome:
//   - (true, false, nil)  -> success; pollWithBackoff returns nil.
//   - (false, false, nil) -> not done yet; keep polling.
//   - (_, true, err)      -> fatal error; returned immediately.
//   - (_, false, err)     -> transient error; retried until maxConsecutiveErrors
//     consecutive transient errors occur, after which err is returned.
//
// On ctx cancellation the context's error is returned, so callers can wrap it
// with errors.Is(err, context.DeadlineExceeded) to add their own message.
func pollWithBackoff(
	ctx context.Context,
	interval time.Duration,
	maxConsecutiveErrors int,
	poll func(ctx context.Context) (done bool, fatal bool, err error),
) error {
	consecutiveErrors := 0

	for {
		done, fatal, err := poll(ctx)
		switch {
		case err != nil && fatal:
			return err
		case err != nil:
			consecutiveErrors++
			if consecutiveErrors >= maxConsecutiveErrors {
				return err
			}
		default:
			consecutiveErrors = 0
			if done {
				return nil
			}
		}

		t := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			t.Stop()
			return ctx.Err()
		case <-t.C:
		}
	}
}
