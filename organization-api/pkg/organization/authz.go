package organization

import (
	"context"
	"errors"
	"fmt"
	"time"

	"connectrpc.com/connect"

	"github.com/fundament-oss/fundament/common/authz"
)

const (
	authzRetryInitialBackoff = 100 * time.Millisecond
	authzRetryMaxBackoff     = 2 * time.Second
	authzRetryBudget         = 8 * time.Second

	// Reads are fired on every page load, one per cluster, and the console
	// shows nothing until they all come back. A denial that is not sync lag
	// must not hold that up, so a read waits a fraction of what a create does
	// and leaves the rest to the caller asking again.
	authzRetryReadBudget = 1 * time.Second
)

var errAuthzUnavailable = errors.New(
	"permissions could not be checked because the authorization service is unavailable; try again in a moment")

// checkPermission performs an OpenFGA authorization check for the current user.
// Returns a connect PermissionDenied error if the check fails, and Unavailable
// when OpenFGA could not be asked.
func (s *Server) checkPermission(ctx context.Context, action authz.Action, resource authz.Object) error {
	if s.authz == nil {
		return nil
	}

	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("user_id missing from context"))
	}

	user := authz.User(userID)

	s.logger.DebugContext(ctx, "check permission", "user", user, "action", action, "resource", resource)

	decision, err := s.authz.Evaluate(ctx, authz.EvaluationRequest{
		Subject:  user,
		Action:   action,
		Resource: resource,
	})
	if err != nil {
		// A caller that went away or ran out of time is not an outage.
		if errors.Is(ctx.Err(), context.Canceled) {
			return connect.NewError(connect.CodeCanceled, ctx.Err())
		}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return connect.NewError(connect.CodeDeadlineExceeded, ctx.Err())
		}

		// The detail names OpenFGA's address, which is for the log and not for
		// whoever made the request. Unavailable tells them asking again may work.
		s.logger.ErrorContext(ctx, "authorization check failed", "error", err, "action", action, "resource", resource)
		return connect.NewError(connect.CodeUnavailable, errAuthzUnavailable)
	}

	if !decision.Decision {
		return connect.NewError(connect.CodePermissionDenied, fmt.Errorf("permission denied"))
	}

	return nil
}

// checkPermissionWithRetry behaves like checkPermission but tolerates the
// eventual consistency of the authz pipeline (DB outbox -> authz-worker ->
// OpenFGA). When the decision is permission denied — which, right after the
// owning cluster/organization was created, usually means the relevant tuple has
// not been synced to OpenFGA yet — it retries with bounded exponential backoff
// so a freshly-synced tuple is picked up. Allow decisions and non-authz errors
// return immediately, and a genuinely unauthorized user fails once the budget is
// exhausted.
func (s *Server) checkPermissionWithRetry(ctx context.Context, action authz.Action, resource authz.Object) error {
	return s.checkPermissionWithBudget(ctx, action, resource, authzRetryBudget)
}

// checkPermissionWithBudget is checkPermissionWithRetry with the time it may
// spend waiting for the tuple to show up spelled out by the caller.
func (s *Server) checkPermissionWithBudget(
	ctx context.Context,
	action authz.Action,
	resource authz.Object,
	budget time.Duration,
) error {
	deadline := time.Now().Add(budget)
	backoff := authzRetryInitialBackoff

	for {
		err := s.checkPermission(ctx, action, resource)
		if err == nil || connect.CodeOf(err) != connect.CodePermissionDenied || time.Now().After(deadline) {
			return err
		}

		s.logger.DebugContext(ctx, "permission denied, retrying after authz sync delay",
			"action", action, "resource", resource, "backoff", backoff)

		select {
		case <-ctx.Done():
			return connect.NewError(connect.CodeCanceled, ctx.Err())
		case <-time.After(backoff):
		}

		backoff *= 2
		if backoff > authzRetryMaxBackoff {
			backoff = authzRetryMaxBackoff
		}
	}
}
