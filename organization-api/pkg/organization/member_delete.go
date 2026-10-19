package organization

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/fundament-oss/fundament/common/authz"
	"github.com/fundament-oss/fundament/common/dbconst"
	"github.com/fundament-oss/fundament/common/rollback"
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *Server) DeleteMember(
	ctx context.Context,
	req *organizationv1.DeleteMemberRequest,
) (*organizationv1.DeleteMemberResponse, error) {
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("user_id missing from context"))
	}

	organizationID, ok := OrganizationIDFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("organization_id missing from context"))
	}

	if err := s.checkPermission(ctx, authz.CanDeleteMember(), authz.Organization(organizationID)); err != nil {
		return nil, err
	}

	id := uuid.MustParse(req.GetId())

	member, err := s.queries.MemberGetByID(ctx, db.MemberGetByIDParams{ID: id})
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("member not found"))
	}

	if member.UserID == userID {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("cannot remove yourself"))
	}

	tx, err := s.db.Pool.Begin(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to begin transaction: %w", err))
	}
	defer rollback.Rollback(ctx, tx, s.logger)

	qtx := s.queries.WithTx(tx)

	// Project memberships go with the organization membership: left behind,
	// they keep their RBAC on the clusters and come back into effect when the
	// user is invited again.
	if err := qtx.ProjectMemberDeleteByUserID(ctx, db.ProjectMemberDeleteByUserIDParams{UserID: member.UserID}); err != nil {
		if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
			if pgErr.Code == pgerrcode.RaiseException && pgErr.Hint == dbconst.HintProjectContainsOneAdmin {
				return nil, connect.NewError(connect.CodeFailedPrecondition, s.lastAdminProjectsError(ctx, member.UserID))
			}
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to remove project memberships: %w", err))
	}

	if err := qtx.MemberDelete(ctx, db.MemberDeleteParams{ID: id}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete member: %w", err))
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to commit transaction: %w", err))
	}

	return organizationv1.DeleteMemberResponse_builder{}.Build(), nil
}

// lastAdminProjectsError names the projects whose only admin is the member
// being removed, so the caller knows where to make someone else admin first.
func (s *Server) lastAdminProjectsError(ctx context.Context, userID uuid.UUID) error {
	const msg = "member is the last admin of projects that need another admin first"

	projects, err := s.queries.ProjectListWhereUserIsLastAdmin(ctx, db.ProjectListWhereUserIsLastAdminParams{UserID: userID})
	if err != nil {
		s.logger.WarnContext(ctx, "list projects blocking member removal", "user_id", userID, "error", err)
		return errors.New(msg)
	}
	if len(projects) == 0 {
		return errors.New(msg)
	}

	names := make([]string, 0, len(projects))
	for _, name := range projects {
		names = append(names, fmt.Sprintf("%q", name))
	}

	return fmt.Errorf("%s: %s", msg, strings.Join(names, ", "))
}
