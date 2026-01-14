package organization

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	"github.com/fundament-oss/fundament/common/authz"
)

// checkPermission performs an OpenFGA authorization check for the current user.
// Returns a connect PermissionDenied error if the check fails.
func (s *OrganizationServer) checkPermission(ctx context.Context, relation, object string) error {
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("user_id missing from context"))
	}

	allowed, err := s.authz.Check(ctx, authz.UserObject(userID), relation, object)
	if err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("authorization check failed: %w", err))
	}

	if !allowed {
		return connect.NewError(connect.CodePermissionDenied, fmt.Errorf("permission denied"))
	}

	return nil
}
