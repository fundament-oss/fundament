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

func (s *Server) DeleteSite(
	ctx context.Context,
	req *connect.Request[dcimv1.DeleteSiteRequest],
) (*connect.Response[emptypb.Empty], error) {
	siteID := uuid.MustParse(req.Msg.GetId())

	rowsAffected, err := s.queries.SiteDelete(ctx, db.SiteDeleteParams{ID: siteID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete site: %w", err))
	}

	if rowsAffected != 1 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("site not found"))
	}

	s.logger.InfoContext(ctx, "site deleted", "site_id", siteID)

	return connect.NewResponse(&emptypb.Empty{}), nil
}
