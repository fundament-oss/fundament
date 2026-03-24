package proxy

import (
	"context"
	"errors"
	"fmt"

	"github.com/fundament-oss/fundament/common/authz"
)

// checkPermission performs an OpenFGA authorization check for the current user.
func (s *Server) checkPermission(ctx context.Context, action authz.Action, resource authz.Object) error {
	if s.authz == nil {
		return nil
	}

	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return fmt.Errorf("user_id missing from context")
	}

	user := authz.User(userID)

	s.logger.DebugContext(ctx, "check permission", "user", user, "action", action, "resource", resource)

	decision, err := s.authz.Evaluate(ctx, authz.EvaluationRequest{
		Subject:  user,
		Action:   action,
		Resource: resource,
	})
	if err != nil {
		return fmt.Errorf("authorization check failed: %w", err)
	}

	if !decision.Decision {
		return errPermissionDenied
	}

	return nil
}

var errPermissionDenied = errors.New("permission denied")
