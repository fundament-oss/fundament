package dcim

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/fundament-oss/fundament/common/auth"
	db "github.com/fundament-oss/fundament/dcim-api/pkg/db/gen"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
)

// lookupCurrentUser resolves the authenticated caller onto their directory
// entry. The JWT subject is an identity-provider reference, not a DCIM user id,
// so it is matched against dcim.users.external_ref; everything else (task
// assignment, note authorship, filtering) uses the internal id this returns.
//
// A caller with a valid token but no directory entry is reported as found=false
// rather than as an error: the roster is provisioned out of band, so being
// absent from it is an ordinary state that some callers can carry on without.
func (s *Server) lookupCurrentUser(ctx context.Context) (row db.UserGetByExternalRefRow, found bool, err error) {
	subject, ok := auth.UserIDFromContext(ctx)
	if !ok {
		return db.UserGetByExternalRefRow{}, false, connect.NewError(connect.CodeUnauthenticated, errors.New("no authenticated user"))
	}

	row, err = s.queries.UserGetByExternalRef(ctx, db.UserGetByExternalRefParams{
		ExternalRef: pgtype.Text{String: subject.String(), Valid: true},
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.UserGetByExternalRefRow{}, false, nil
		}
		return db.UserGetByExternalRefRow{}, false, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get current user: %w", err))
	}
	return row, true, nil
}

// currentUser is lookupCurrentUser for the callers that cannot proceed without
// a directory entry, turning "not in the roster" into a NotFound.
func (s *Server) currentUser(ctx context.Context) (db.UserGetByExternalRefRow, error) {
	row, found, err := s.lookupCurrentUser(ctx)
	if err != nil {
		return db.UserGetByExternalRefRow{}, err
	}
	if !found {
		return db.UserGetByExternalRefRow{}, connect.NewError(connect.CodeNotFound, errors.New("no directory entry for the authenticated user"))
	}
	return row, nil
}

func (s *Server) GetCurrentUser(
	ctx context.Context,
	_ *connect.Request[dcimv1.GetCurrentUserRequest],
) (*connect.Response[dcimv1.GetCurrentUserResponse], error) {
	row, err := s.currentUser(ctx)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(dcimv1.GetCurrentUserResponse_builder{
		User: userToProto(row.ID, row.Name, row.Email),
	}.Build()), nil
}
