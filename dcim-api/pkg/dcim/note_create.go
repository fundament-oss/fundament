package dcim

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/fundament-oss/fundament/common/dbconst"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
)

func (s *Server) CreateNote(
	ctx context.Context,
	req *connect.Request[dcimv1.CreateNoteRequest],
) (*connect.Response[dcimv1.CreateNoteResponse], error) {
	entityID := uuid.MustParse(req.Msg.GetEntityId())
	params, err := noteEntityToCreateParams(req.Msg.GetEntityType(), entityID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	// Attribute the note to the authenticated caller, never to a client-supplied
	// value, so the author cannot be spoofed. The roster is provisioned out of
	// band, so a caller who is not in it writes an unattributed note rather than
	// being refused — note-taking must not depend on directory coverage.
	author, found, err := s.lookupCurrentUser(ctx)
	if err != nil {
		return nil, err
	}

	params.Body = req.Msg.GetBody()
	if found {
		params.CreatedByID = pgtype.UUID{Bytes: author.ID, Valid: true}
	}

	id, err := s.queries.NoteCreate(ctx, params)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch pgErr.ConstraintName {
			case dbconst.ConstraintDcimNotesFkCreatedBy:
				return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("note author not found"))
			}
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create note: %w", err))
	}

	s.logger.InfoContext(ctx, "note created", "note_id", id)

	return connect.NewResponse(dcimv1.CreateNoteResponse_builder{
		NoteId: id.String(),
	}.Build()), nil
}
