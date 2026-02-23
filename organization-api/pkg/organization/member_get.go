package organization

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/types/known/timestamppb"

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
		member = memberFromGetByIDRow(&row)

	case *organizationv1.GetMemberRequest_UserId:
		userID := uuid.MustParse(lookup.UserId)
		row, err := s.queries.MemberGetByUserID(ctx, db.MemberGetByUserIDParams{UserID: userID})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("member not found"))
			}
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get member: %w", err))
		}
		member = memberFromGetByUserIDRow(&row)

	default:
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("either id or user_id must be provided"))
	}

	return connect.NewResponse(&organizationv1.GetMemberResponse{
		Member: member,
	}), nil
}

func memberFromGetByIDRow(m *db.MemberGetByIDRow) *organizationv1.Member {
	member := &organizationv1.Member{
		Id:         m.ID.String(),
		UserId:     m.UserID.String(),
		Name:       m.Name,
		Permission: string(m.Permission),
		Status:     string(m.Status),
		Created:    timestamppb.New(m.Created.Time),
	}

	if m.ExternalRef.Valid {
		member.ExternalRef = &m.ExternalRef.String
	}

	if m.Email.Valid {
		member.Email = &m.Email.String
	}

	return member
}

func memberFromGetByUserIDRow(m *db.MemberGetByUserIDRow) *organizationv1.Member {
	member := &organizationv1.Member{
		Id:         m.ID.String(),
		UserId:     m.UserID.String(),
		Name:       m.Name,
		Permission: string(m.Permission),
		Status:     string(m.Status),
		Created:    timestamppb.New(m.Created.Time),
	}

	if m.ExternalRef.Valid {
		member.ExternalRef = &m.ExternalRef.String
	}

	if m.Email.Valid {
		member.Email = &m.Email.String
	}

	return member
}
