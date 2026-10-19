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
// entry. The JWT subject is the internal dcim.users.id: dcim-authn-api
// provisions the entry just-in-time at login and mints the token with the id
// it resolved, so a lookup by id is all that remains here.
//
// A caller with a valid token but no directory entry is reported as found=false
// rather than as an error: a user can be soft-deleted while their token is
// still live, which is an ordinary state that some callers can carry on
// without.
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
