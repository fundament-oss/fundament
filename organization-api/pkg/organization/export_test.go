package organization

import (
	"context"
	"time"

	"github.com/google/uuid"

	authzv1 "github.com/fundament-oss/fundament/authz-api/pkg/proto/gen/v1"
	"github.com/fundament-oss/fundament/common/authz"
)

type decisionFunc func(authz.EvaluationRequest) (authz.Decision, error)

//nolint:gocritic // mirrors authz.Client.Evaluate
func (f decisionFunc) Evaluate(_ context.Context, req authz.EvaluationRequest) (authz.Decision, error) {
	return f(req)
}

// WithDecisions replaces OpenFGA with fn.
func WithDecisions(fn func(authz.EvaluationRequest) (authz.Decision, error)) Option {
	return func(s *Server) {
		s.authz = decisionFunc(fn)
	}
}

func (s *Server) CheckPermissionWithRetry(ctx context.Context, action authzv1.Action, id uuid.UUID) error {
	return s.checkPermissionWithRetry(ctx, action, id)
}

func (s *Server) CheckPermissionWithBudget(ctx context.Context, action authzv1.Action, id uuid.UUID, budget time.Duration) error {
	return s.checkPermissionWithBudget(ctx, action, id, budget)
}
