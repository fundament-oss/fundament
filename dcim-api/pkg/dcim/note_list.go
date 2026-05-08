package dcim

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
)

func (s *Server) ListNotes(
	ctx context.Context,
	req *connect.Request[dcimv1.ListNotesRequest],
) (*connect.Response[dcimv1.ListNotesResponse], error) {
	entityID := uuid.MustParse(req.Msg.GetEntityId())
	params := noteEntityToListParams(req.Msg.GetEntityType(), entityID)

	rows, err := s.queries.NoteList(ctx, params)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list notes: %w", err))
	}

	notes := make([]*dcimv1.Note, 0, len(rows))
	for _, row := range rows {
		notes = append(notes, noteFromListRow(&row))
	}

	return connect.NewResponse(dcimv1.ListNotesResponse_builder{
		Notes: notes,
	}.Build()), nil
}
