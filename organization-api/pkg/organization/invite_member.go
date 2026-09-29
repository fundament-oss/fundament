package organization

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/fundament-oss/fundament/common/authz"
	"github.com/fundament-oss/fundament/common/dbconst"
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *Server) InviteMember(
	ctx context.Context,
	req *organizationv1.InviteMemberRequest,
) (*organizationv1.InviteMemberResponse, error) {
	organizationID, ok := OrganizationIDFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("organization_id missing from context"))
	}

	if err := s.checkPermission(ctx, authz.CanInviteMember(), authz.Organization(organizationID)); err != nil {
		return nil, err
	}

	email := req.GetEmail()
	permission := req.GetPermission()

	// Find existing user by email, or create a new one
	var userID uuid.UUID

	existingUser, err := s.queries.UserFindByEmail(ctx, db.UserFindByEmailParams{Email: email})
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to look up user: %w", err))
		}

		newUser, err := s.queries.UserCreate(ctx, db.UserCreateParams{Email: email})
		switch {
		case err == nil:
			userID = newUser.ID
		case isConstraintViolation(err, dbconst.ConstraintUsersUqEmail):
			// Someone registered the address between the lookup and the insert
			// (a concurrent invite, or that person's first sign-in). One live
			// user per address is the rule, so theirs is the one to invite.
			existingUser, err := s.queries.UserFindByEmail(ctx, db.UserFindByEmailParams{Email: email})
			if err != nil {
				return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to look up user: %w", err))
			}
			userID = existingUser.ID
		default:
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create invited user: %w", err))
		}
	} else {
		userID = existingUser.ID
	}

	// Create the organization membership
	membershipRow, err := s.queries.InviteCreateMembership(ctx, db.InviteCreateMembershipParams{
		OrganizationID: organizationID,
		UserID:         userID,
		Permission:     permission,
	})
	if err != nil {
		if isConstraintViolation(err, dbconst.ConstraintOrganizationsUsersUqUser) {
			return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("user is already a member of this organization"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create membership: %w", err))
	}

	return organizationv1.InviteMemberResponse_builder{
		InvitationId: membershipRow.ID.String(),
	}.Build(), nil
}

// isConstraintViolation reports whether err is Postgres refusing a statement
// on the named constraint or unique index.
func isConstraintViolation(err error, constraint string) bool {
	pgErr, ok := errors.AsType[*pgconn.PgError](err)
	return ok && pgErr.ConstraintName == constraint
}
