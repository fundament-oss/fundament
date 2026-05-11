package dcim

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
)

func (s *Server) ListSites(
	ctx context.Context,
	req *connect.Request[dcimv1.ListSitesRequest],
) (*connect.Response[dcimv1.ListSitesResponse], error) {
	rows, err := s.queries.SiteList(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list sites: %w", err))
	}

	sites := make([]*dcimv1.Site, 0, len(rows))
	for _, row := range rows {
		sites = append(sites, siteFromListRow(&row))
	}

	return connect.NewResponse(dcimv1.ListSitesResponse_builder{
		Sites: sites,
	}.Build()), nil
}
