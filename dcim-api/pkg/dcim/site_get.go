package dcim

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	db "github.com/fundament-oss/fundament/dcim-api/pkg/db/gen"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
)

func (s *Server) GetSite(
	ctx context.Context,
	req *connect.Request[dcimv1.GetSiteRequest],
) (*connect.Response[dcimv1.GetSiteResponse], error) {
	siteID := uuid.MustParse(req.Msg.GetId())

	site, err := s.queries.SiteGetByID(ctx, db.SiteGetByIDParams{ID: siteID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("site not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get site: %w", err))
	}

	return connect.NewResponse(dcimv1.GetSiteResponse_builder{
		Site: siteFromRow(&site),
	}.Build()), nil
}
