package dcim

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/fundament-oss/fundament/common/dbconst"
	db "github.com/fundament-oss/fundament/dcim-api/pkg/db/gen"

	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
)

func (s *Server) CreateDesign(
	ctx context.Context,
	req *connect.Request[dcimv1.CreateDesignRequest],
) (*connect.Response[dcimv1.CreateDesignResponse], error) {
	params := db.LogicalDesignCreateParams{
		Name: req.Msg.GetName(),
	}

	if req.Msg.GetDescription() != "" {
		params.Description = pgtype.Text{String: req.Msg.GetDescription(), Valid: true}
	}

	id, err := s.queries.LogicalDesignCreate(ctx, params)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.ConstraintName == dbconst.ConstraintLogicalDesignsUqName {
			return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("design with this name already exists"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create design: %w", err))
	}

	s.logger.InfoContext(ctx, "design created", "design_id", id)

	return connect.NewResponse(dcimv1.CreateDesignResponse_builder{
		DesignId: id.String(),
	}.Build()), nil
}
