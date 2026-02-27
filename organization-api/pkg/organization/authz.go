package organization

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	"github.com/fundament-oss/fundament/common/authz"
)

// checkPermission performs an OpenFGA authorization check for the current user.
// Returns a connect PermissionDenied error if the check fails.
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
