package dcim

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
)

func (s *Server) ListUsers(
	ctx context.Context,
	req *connect.Request[dcimv1.ListUsersRequest],
) (*connect.Response[dcimv1.ListUsersResponse], error) {
	rows, err := s.queries.UserList(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list users: %w", err))
	}

	users := make([]*dcimv1.User, 0, len(rows))
	for _, row := range rows {
		users = append(users, userToProto(row.ID, row.Name, row.Email))
	}

	return connect.NewResponse(dcimv1.ListUsersResponse_builder{
		Users: users,
	}.Build()), nil
}
