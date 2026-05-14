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

func (s *Server) CreateSite(
	ctx context.Context,
	req *connect.Request[dcimv1.CreateSiteRequest],
) (*connect.Response[dcimv1.CreateSiteResponse], error) {
	params := db.SiteCreateParams{
		Name: req.Msg.GetName(),
	}

	if req.Msg.HasAddress() {
		params.Address = pgtype.Text{String: req.Msg.GetAddress(), Valid: true}
	}

	id, err := s.queries.SiteCreate(ctx, params)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.ConstraintName == dbconst.ConstraintSitesUqName {
			return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("site with this name already exists"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create site: %w", err))
	}

	s.logger.InfoContext(ctx, "site created", "site_id", id)

	return connect.NewResponse(dcimv1.CreateSiteResponse_builder{
		SiteId: id.String(),
	}.Build()), nil
}
