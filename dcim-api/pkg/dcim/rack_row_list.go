package dcim

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/fundament-oss/fundament/dcim-api/pkg/db/gen"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
)

func (s *Server) ListRackRows(
	ctx context.Context,
	req *connect.Request[dcimv1.ListRackRowsRequest],
) (*connect.Response[dcimv1.ListRackRowsResponse], error) {
	params := db.RackRowListParams{}

	if req.Msg.HasRoomId() {
		roomID := uuid.MustParse(req.Msg.GetRoomId())
		params.RoomID = pgtype.UUID{Bytes: roomID, Valid: true}
	}

	rows, err := s.queries.RackRowList(ctx, params)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list rack rows: %w", err))
	}

	rackRows := make([]*dcimv1.RackRow, 0, len(rows))
	for _, row := range rows {
		rackRows = append(rackRows, rackRowFromListRow(&row))
	}

	return connect.NewResponse(dcimv1.ListRackRowsResponse_builder{
		RackRows: rackRows,
	}.Build()), nil
}
