package organization

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/fundament-oss/fundament/common/dbconst"
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *Server) GetMember(
	ctx context.Context,
	req *connect.Request[organizationv1.GetMemberRequest],
) (*connect.Response[organizationv1.GetMemberResponse], error) {
	var member *organizationv1.Member

	switch req.Msg.WhichLookup() {
	case organizationv1.GetMemberRequest_Id_case:
		id := uuid.MustParse(req.Msg.GetId())
		row, err := s.queries.MemberGetByID(ctx, db.MemberGetByIDParams{ID: id})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("member not found"))
			}
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get member: %w", err))
		}
		member = buildMember(row.ID, row.UserID, row.Name, row.ExternalRef, row.Email, row.Permission, row.Status, row.Created)

	case organizationv1.GetMemberRequest_UserId_case:
		userID := uuid.MustParse(req.Msg.GetUserId())
		row, err := s.queries.MemberGetByUserID(ctx, db.MemberGetByUserIDParams{UserID: userID})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("member not found"))
			}
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get member: %w", err))
		}
		member = buildMember(row.ID, row.UserID, row.Name, row.ExternalRef, row.Email, row.Permission, row.Status, row.Created)

	default:
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("either id or user_id must be provided"))
	}

	return connect.NewResponse(organizationv1.GetMemberResponse_builder{
		Member: member,
	}.Build()), nil
}

func buildMember(
	id, userID uuid.UUID,
	name string,
	externalRef, email pgtype.Text,
	permission dbconst.OrganizationsUserPermission,
	status dbconst.OrganizationsUserStatus,
	created pgtype.Timestamptz,
) *organizationv1.Member {
	member := organizationv1.Member_builder{
		Id:         id.String(),
		UserId:     userID.String(),
		Name:       name,
		Permission: string(permission),
		Status:     string(status),
		Created:    timestamppb.New(created.Time),
	}.Build()

	if externalRef.Valid {
		member.SetExternalRef(externalRef.String)
	}

	if email.Valid {
		member.SetEmail(email.String)
	}

	return member
}
