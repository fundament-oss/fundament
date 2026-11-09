package dcimauthn

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	db "github.com/fundament-oss/fundament/dcim-authn-api/pkg/db/gen"
	dcimauthnv1 "github.com/fundament-oss/fundament/dcim-authn-api/pkg/proto/gen/dcimauthn/v1"
)

// GetUserInfo returns the authenticated caller's directory entry, resolved
// against dcim.users rather than echoed from the claims. The frontend probes
// this first and only refreshes (and then re-logs) when it fails, so a session
// whose subject no longer resolves must fail here; a directory outage answers
// Unavailable so the frontend can tell a transient failure apart from being
// signed out.
func (s *Server) GetUserInfo(
	ctx context.Context,
	_ *dcimauthnv1.GetUserInfoRequest,
) (*dcimauthnv1.GetUserInfoResponse, error) {
	callInfo, ok := connect.CallInfoForHandlerContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeInternal, errors.New("missing call info in context"))
	}

	claims, err := s.validator.Validate(callInfo.RequestHeader())
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}

	row, err := s.queries.UserGetByID(ctx, db.UserGetByIDParams{ID: claims.UserID()})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("invalid or expired token"))
		}
		s.logger.Error("failed to resolve user", "error", err)
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("temporarily unavailable"))
	}

	return dcimauthnv1.GetUserInfoResponse_builder{
		User: dcimauthnv1.User_builder{
			Id:   row.ID.String(),
			Name: row.Name,
		}.Build(),
	}.Build(), nil
}
