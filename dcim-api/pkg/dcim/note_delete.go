package dcim

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/emptypb"

	db "github.com/fundament-oss/fundament/dcim-api/pkg/db/gen"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
)

func (s *Server) DeleteNote(
	ctx context.Context,
	req *connect.Request[dcimv1.DeleteNoteRequest],
) (*connect.Response[emptypb.Empty], error) {
	noteID := uuid.MustParse(req.Msg.GetId())

	rowsAffected, err := s.queries.NoteDelete(ctx, db.NoteDeleteParams{ID: noteID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete note: %w", err))
	}

	if rowsAffected != 1 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("note not found"))
	}

	s.logger.InfoContext(ctx, "note deleted", "note_id", noteID)

	return connect.NewResponse(&emptypb.Empty{}), nil
}
