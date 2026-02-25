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

	switch lookup := req.Msg.Lookup.(type) {
	case *organizationv1.GetMemberRequest_Id:
		id := uuid.MustParse(lookup.Id)
		row, err := s.queries.MemberGetByID(ctx, db.MemberGetByIDParams{ID: id})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("member not found"))
			}
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get member: %w", err))
		}
		member = buildMember(row.ID, row.UserID, row.Name, row.ExternalRef, row.Email, row.Permission, row.Status, row.Created)

	case *organizationv1.GetMemberRequest_UserId:
		userID := uuid.MustParse(lookup.UserId)
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

	return connect.NewResponse(&organizationv1.GetMemberResponse{
		Member: member,
	}), nil
}

func buildMember(
	id, userID uuid.UUID,
	name string,
	externalRef, email pgtype.Text,
	permission dbconst.OrganizationsUserPermission,
	status dbconst.OrganizationsUserStatus,
	created pgtype.Timestamptz,
) *organizationv1.Member {
	member := &organizationv1.Member{
		Id:         id.String(),
		UserId:     userID.String(),
		Name:       name,
		Permission: string(permission),
		Status:     string(status),
		Created:    timestamppb.New(created.Time),
	}

	if externalRef.Valid {
		member.ExternalRef = &externalRef.String
	}

	if email.Valid {
		member.Email = &email.String
	}

	return member
}
