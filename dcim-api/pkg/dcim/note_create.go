package dcim

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
)

func (s *Server) CreateNote(
	ctx context.Context,
	req *connect.Request[dcimv1.CreateNoteRequest],
) (*connect.Response[dcimv1.CreateNoteResponse], error) {
	entityID := uuid.MustParse(req.Msg.GetEntityId())
	params := noteEntityToCreateParams(req.Msg.GetEntityType(), entityID)

	params.Body = req.Msg.GetBody()
	params.CreatedBy = pgtype.Text{String: req.Msg.GetCreatedBy(), Valid: true}

	id, err := s.queries.NoteCreate(ctx, params)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create note: %w", err))
	}

	s.logger.InfoContext(ctx, "note created", "note_id", id)

	return connect.NewResponse(dcimv1.CreateNoteResponse_builder{
		NoteId: id.String(),
	}.Build()), nil
}
