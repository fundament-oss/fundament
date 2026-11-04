package authzapi

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	authzv1 "github.com/fundament-oss/fundament/authz-api/pkg/proto/gen/v1"
	"github.com/fundament-oss/fundament/common/auth"
	"github.com/fundament-oss/fundament/common/authz"
	"github.com/fundament-oss/fundament/common/authz/actions"
)

// CheckPermissions reports, in request order, whether the caller may take each
// action. An object the caller has no relation to is simply not allowed.
func (s *Server) CheckPermissions(
	ctx context.Context,
	req *authzv1.CheckPermissionsRequest,
) (*authzv1.CheckPermissionsResponse, error) {
	userID, ok := auth.UserIDFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("user_id missing from context"))
	}

	checks := req.GetChecks()
	resp := authzv1.CheckPermissionsResponse_builder{Allowed: make([]bool, 0, len(checks))}.Build()
	if len(checks) == 0 {
		return resp, nil
	}

	evals := make([]authz.EvaluationRequest, 0, len(checks))
	for _, check := range checks {
		evals = append(evals, authz.EvaluationRequest{
			Subject:  authz.User(userID),
			Action:   actions.Relation(check.GetAction()),
			Resource: actions.Object(check.GetAction(), uuid.MustParse(check.GetObjectId())),
		})
	}

	result, err := s.authz.Evaluations(ctx, authz.EvaluationsRequest{Evaluations: evals})
	if err != nil {
		s.logger.ErrorContext(ctx, "openfga batch check failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("permission check failed"))
	}

	allowed := make([]bool, 0, len(result.Evaluations))
	for _, d := range result.Evaluations {
		allowed = append(allowed, d.Decision)
	}
	resp.SetAllowed(allowed)

	return resp, nil
}
