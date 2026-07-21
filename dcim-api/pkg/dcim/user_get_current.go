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

// KNOWN GAP — dcim.users has no provisioning path outside local development.
//
// The table is populated only by db/testdata/030_0101-content.sql, and
// fun_dcim_api holds SELECT on it and nothing more (migration 030), so no
// service can write it. There is no sync from dex or dcim-authn-api either.
// In any environment that is not seeded, therefore:
//
//   - GetCurrentUser answers NotFound for every caller, so the technician page
//     reports "your account is not in the technician directory" to everyone;
//   - the admin board's assignee picker and bulk-assign menu are empty, and no
//     task can be assigned to anybody;
//   - every note is written unattributed, since CreateNote resolves the author
//     through this same lookup.
//
// None of that is a bug in the code below — it behaves correctly for an empty
// roster. It is a deployment prerequisite that does not exist yet. Closing it
// needs a decision on where the roster comes from (just-in-time on first
// authenticated call, explicit write RPCs, or a sync job), plus a migration
// granting fun_dcim_api the writes that choice implies. Deliberately deferred;
// do not read the empty-roster handling here as evidence it is solved.
//
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
		User: userToProtoWithEmail(row.ID, row.Name, row.Email),
	}.Build()), nil
}
