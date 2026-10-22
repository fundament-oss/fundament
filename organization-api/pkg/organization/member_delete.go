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

	// Leaving the organization ends the user's project memberships too, in the
	// same transaction: project membership is only ever granted to organization
	// members, and a later re-invite must not silently restore old project roles.
	tx, err := s.db.Pool.Begin(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to begin transaction: %w", err))
	}
	defer rollback.Rollback(ctx, tx, s.logger)

	qtx := s.queries.WithTx(tx)

	if err = qtx.MemberDelete(ctx, db.MemberDeleteParams{ID: id}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete member: %w", err))
	}

	_, err = qtx.ProjectMemberDeleteByUserInOrganization(ctx, db.ProjectMemberDeleteByUserInOrganizationParams{UserID: member.UserID})
	if err != nil {
		if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
			if pgErr.Code == pgerrcode.RaiseException && pgErr.Hint == dbconst.HintProjectContainsOneAdmin {
				return nil, connect.NewError(connect.CodeFailedPrecondition, s.memberLastAdminError(ctx, member.UserID))
			}
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete project memberships: %w", err))
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to commit transaction: %w", err))
	}

	return organizationv1.DeleteMemberResponse_builder{}.Build(), nil
}

// memberLastAdminError names the projects that would be left without an admin
// if the user were removed, so the caller knows where to assign another one.
func (s *Server) memberLastAdminError(ctx context.Context, userID uuid.UUID) error {
	projects, err := s.queries.ProjectListWhereUserIsLastAdmin(ctx, db.ProjectListWhereUserIsLastAdminParams{UserID: userID})
	if err != nil || len(projects) == 0 {
		return fmt.Errorf("cannot remove the last admin of a project; assign another project admin first")
	}

	names := make([]string, 0, len(projects))
	for _, p := range projects {
		names = append(names, p.Name)
	}
	return fmt.Errorf("cannot remove the last admin of project(s) %s; assign another project admin first", strings.Join(names, ", "))
}
