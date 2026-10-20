package dcim

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	"github.com/fundament-oss/fundament/common/auth"
	db "github.com/fundament-oss/fundament/dcim-api/pkg/db/gen"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
)

// lookupCurrentUser resolves the authenticated caller onto their directory
// entry by the JWT subject, their dcim.users.id.
//
// The auth interceptor already requires a live entry, so found=false covers
// only the sliver where the user is soft-deleted between the interceptor's
// check and this read; callers that can proceed without attribution do.
func (s *Server) lookupCurrentUser(ctx context.Context) (row db.UserGetByIDRow, found bool, err error) {
	subject, ok := auth.UserIDFromContext(ctx)
	if !ok {
		return db.UserGetByIDRow{}, false, connect.NewError(connect.CodeUnauthenticated, errors.New("no authenticated user"))
	}

	row, err = s.queries.UserGetByID(ctx, db.UserGetByIDParams{
		ID: subject,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.UserGetByIDRow{}, false, nil
		}
		return db.UserGetByIDRow{}, false, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get current user: %w", err))
	}
	return row, true, nil
}

// currentUser is lookupCurrentUser for the callers that cannot proceed without
// a directory entry, turning "not in the roster" into a NotFound.
func (s *Server) currentUser(ctx context.Context) (db.UserGetByIDRow, error) {
	row, found, err := s.lookupCurrentUser(ctx)
	if err != nil {
		return db.UserGetByIDRow{}, err
	}
	if !found {
		return db.UserGetByIDRow{}, connect.NewError(connect.CodeNotFound, errors.New("no directory entry for the authenticated user"))
	}
	return row, nil
}

func (s *Server) GetCurrentUser(
	ctx context.Context,
	_ *dcimv1.GetCurrentUserRequest,
) (*dcimv1.GetCurrentUserResponse, error) {
	row, err := s.currentUser(ctx)
	if err != nil {
		return nil, err
	}

	return dcimv1.GetCurrentUserResponse_builder{
		User: userToProtoWithEmail(row.ID, row.Name, row.Email),
	}.Build(), nil
}
