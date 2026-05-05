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
	db "github.com/fundament-oss/fundament/dcim-api/pkg/db/gen"

	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
)

func (s *Server) UpdateSite(
	ctx context.Context,
	req *connect.Request[dcimv1.UpdateSiteRequest],
) (*connect.Response[dcimv1.UpdateSiteResponse], error) {
	siteID := uuid.MustParse(req.Msg.GetId())

	params := db.SiteUpdateParams{
		ID: siteID,
	}

	if req.Msg.HasName() {
		params.Name = pgtype.Text{String: req.Msg.GetName(), Valid: true}
	}

	if req.Msg.HasAddress() {
		params.Address = pgtype.Text{String: req.Msg.GetAddress(), Valid: true}
	}

	rowsAffected, err := s.queries.SiteUpdate(ctx, params)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.ConstraintName == dbconst.ConstraintSitesUqName {
			return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("site with this name already exists"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update site: %w", err))
	}

	if rowsAffected != 1 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("site not found"))
	}

	s.logger.InfoContext(ctx, "site updated", "site_id", siteID)

	return connect.NewResponse(dcimv1.UpdateSiteResponse_builder{}.Build()), nil
}
