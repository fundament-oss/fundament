package organization

import (
	"context"
	"errors"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	authzv1 "github.com/fundament-oss/fundament/authz-api/pkg/proto/gen/v1"
	"github.com/fundament-oss/fundament/common/authz"
	"github.com/fundament-oss/fundament/common/authz/actions"
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
)

const (
	authzRetryInitialBackoff = 100 * time.Millisecond
	authzRetryMaxBackoff     = 2 * time.Second
	authzRetryBudget         = 8 * time.Second

	// Reads are fired on every page load, one per cluster, and the console
	// shows nothing until they all come back, so a read waits a fraction of
	// what a create does and leaves the rest to the caller asking again.
	authzRetryReadBudget = 1 * time.Second
)

type authzEvaluator interface {
	Evaluate(ctx context.Context, req authz.EvaluationRequest) (authz.Decision, error)
}

// checkPermission performs an OpenFGA authorization check for the current user.
// A denial is a PermissionDenied error carrying a PermissionDeniedDetail.
func (s *Server) checkPermission(ctx context.Context, action authzv1.Action, id uuid.UUID) error {
	if s.authz == nil {
		return nil
	}

	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("user_id missing from context"))
	}

	user := authz.User(userID)
	relation := actions.Relation(action)
	resource := actions.Object(action, id)

	s.logger.DebugContext(ctx, "check permission", "user", user, "action", action.String())

	decision, err := s.authz.Evaluate(ctx, authz.EvaluationRequest{
		Subject:  user,
		Action:   relation,
		Resource: resource,
	})
	if err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("authorization check failed: %w", err))
	}

	if !decision.Decision {
		return s.permissionDenied(ctx, action)
	}

	return nil
}

func (s *Server) permissionDenied(ctx context.Context, action authzv1.Action) error {
	denied := connect.NewError(connect.CodePermissionDenied, errors.New("permission denied"))

	detail, err := connect.NewErrorDetail(authzv1.PermissionDeniedDetail_builder{
		Action:       action,
		RequiredRole: actions.Role(action),
	}.Build())
	if err != nil {
		s.logger.ErrorContext(ctx, "building permission denied detail", "error", err)
		return denied
	}
	denied.AddDetail(detail)

	return denied
}

// checkPermissionWithRetry behaves like checkPermission but tolerates the
// eventual consistency of the authz pipeline (DB outbox -> authz-worker ->
// OpenFGA). A denial is retried with bounded exponential backoff only while an
// outbox row the decision depends on is still pending or retrying; otherwise it
// returns at once. Only cluster and project objects are supported, and any other
// panics.
func (s *Server) checkPermissionWithRetry(ctx context.Context, action authzv1.Action, id uuid.UUID) error {
	return s.checkPermissionWithBudget(ctx, action, id, authzRetryBudget)
}

// checkPermissionWithBudget is checkPermissionWithRetry with the time it may
// spend waiting for a sync spelled out by the caller.
func (s *Server) checkPermissionWithBudget(ctx context.Context, action authzv1.Action, id uuid.UUID, budget time.Duration) error {
	syncPending := s.syncPendingQuery(action, id)

	deadline := time.Now().Add(budget)
	backoff := authzRetryInitialBackoff

	for {
		err := s.checkPermission(ctx, action, id)
		if err == nil || connect.CodeOf(err) != connect.CodePermissionDenied || !time.Now().Before(deadline) {
			return err
		}

		pending, queryErr := syncPending(ctx)
		if queryErr != nil {
			s.logger.WarnContext(ctx, "reading authz outbox after denial", "error", queryErr)
			return err
		}
		if !pending {
			// A row completed after the check above has already written its tuple.
			return s.checkPermission(ctx, action, id)
		}

		s.logger.DebugContext(ctx, "permission denied while authz sync is pending, retrying",
			"action", action, "id", id, "backoff", backoff)

		select {
		case <-ctx.Done():
			return connect.NewError(connect.CodeCanceled, ctx.Err())
		case <-time.After(min(backoff, time.Until(deadline))):
		}

		backoff = min(backoff*2, authzRetryMaxBackoff)
	}
}

// syncPendingQuery returns the outbox query for what a decision on id depends on.
func (s *Server) syncPendingQuery(action authzv1.Action, id uuid.UUID) func(context.Context) (bool, error) {
	switch objectType := actions.Object(action, id).Type; objectType {
	case authz.ObjectTypeCluster:
		return func(ctx context.Context) (bool, error) {
			userID, _ := UserIDFromContext(ctx)
			//nolint:wrapcheck // logged by the caller
			return s.queries.AuthzOutboxPendingForCluster(ctx, db.AuthzOutboxPendingForClusterParams{ClusterID: id, UserID: userID})
		}
	case authz.ObjectTypeProject:
		return func(ctx context.Context) (bool, error) {
			userID, _ := UserIDFromContext(ctx)
			//nolint:wrapcheck // logged by the caller
			return s.queries.AuthzOutboxPendingForProject(ctx, db.AuthzOutboxPendingForProjectParams{ProjectID: id, UserID: userID})
		}
	default:
		panic(fmt.Sprintf("permission retry on unsupported object type %q for %s", objectType, action))
	}
}
