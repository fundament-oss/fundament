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

// GetCurrentUser resolves the authenticated caller onto their directory entry.
// The JWT subject is an identity-provider reference, not a DCIM user id, so it
// is matched against dcim.users.external_ref; everything else (task assignment,
// filtering) uses the internal id this returns.
func (s *Server) GetCurrentUser(
	ctx context.Context,
	_ *connect.Request[dcimv1.GetCurrentUserRequest],
) (*connect.Response[dcimv1.GetCurrentUserResponse], error) {
	subject, ok := auth.UserIDFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("no authenticated user"))
	}

	row, err := s.queries.UserGetByExternalRef(ctx, db.UserGetByExternalRefParams{
		ExternalRef: pgtype.Text{String: subject.String(), Valid: true},
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("no directory entry for the authenticated user"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get current user: %w", err))
	}

	return connect.NewResponse(dcimv1.GetCurrentUserResponse_builder{
		User: userToProto(row.ID, row.Name, row.Email),
	}.Build()), nil
}
