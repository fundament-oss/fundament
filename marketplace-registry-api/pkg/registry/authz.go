package registry

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"

	"github.com/fundament-oss/fundament/common/authz"
)

const (
	authzRetryInitialBackoff = 100 * time.Millisecond
	authzRetryMaxBackoff     = 2 * time.Second
	authzRetryBudget         = 8 * time.Second
)

// checkPermission performs an OpenFGA authorization check for the current
// user, the same layering organization-api has: OpenFGA answers "may this
// user", RLS stays the tenancy backstop underneath. A nil client skips the
// check, which is how the integration tests run.
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
		return connect.NewError(connect.CodeInternal, fmt.Errorf("authorization check failed: %w", err))
	}

	if !decision.Decision {
		return connect.NewError(connect.CodePermissionDenied, fmt.Errorf("permission denied"))
	}

	return nil
}

// checkPermissionWithRetry tolerates the eventual consistency of the authz
// pipeline (DB outbox -> authz-worker -> OpenFGA). A plugin's ownership tuple
// syncs asynchronously after CreatePlugin, and pushing a version right after
// creating the listing is the normal CLI flow, so a denial here usually means
// the tuple has not landed yet. Retries with bounded backoff; a genuinely
// unauthorized user still fails once the budget is exhausted.
func (s *Server) checkPermissionWithRetry(ctx context.Context, action authz.Action, resource authz.Object) error {
	deadline := time.Now().Add(authzRetryBudget)
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
