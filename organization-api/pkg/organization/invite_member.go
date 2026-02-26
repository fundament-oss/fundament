package organization

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/fundament-oss/fundament/common/authz"
	"github.com/fundament-oss/fundament/common/dbconst"
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *Server) InviteMember(
	ctx context.Context,
	req *connect.Request[organizationv1.InviteMemberRequest],
) (*connect.Response[organizationv1.InviteMemberResponse], error) {
	organizationID, ok := OrganizationIDFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("organization_id missing from context"))
	}

	if err := s.checkPermission(ctx, authz.CanInviteMember(), authz.Organization(organizationID)); err != nil {
		return nil, err
	}

	email := req.Msg.GetEmail()
	permission := req.Msg.GetPermission()

	// Find existing user by email, or create a new one
	var userID uuid.UUID

	existingUser, err := s.queries.UserFindByEmail(ctx, db.UserFindByEmailParams{Email: email})
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to look up user: %w", err))
		}

		newUser, err := s.queries.UserCreate(ctx, db.UserCreateParams{Email: email})
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create invited user: %w", err))
		}

		userID = newUser.ID
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
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.ConstraintName == dbconst.ConstraintOrganizationsUsersUqUser {
			return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("user is already a member of this organization"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create membership: %w", err))
	}

	return connect.NewResponse(organizationv1.InviteMemberResponse_builder{
		Member: memberFromInviteRow(email, &membershipRow),
	}.Build()), nil
}

func memberFromInviteRow(email string, m *db.InviteCreateMembershipRow) *organizationv1.Member {
	member := organizationv1.Member_builder{
		Id:         m.ID.String(),
		UserId:     m.UserID.String(),
		Name:       email,
		Email:      &email,
		Permission: string(m.Permission),
		Status:     string(m.Status),
		Created:    timestamppb.New(m.Created.Time),
	}.Build()
	return member
}
