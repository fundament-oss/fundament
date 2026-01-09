package organization

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	"github.com/fundament-oss/fundament/organization-api/pkg/organization/adapter"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *OrganizationServer) ListPlugins(
	ctx context.Context,
	req *connect.Request[organizationv1.ListPluginsRequest],
) (*connect.Response[organizationv1.ListPluginsResponse], error) {
	plugins, err := s.queries.PluginList(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list plugins: %w", err))
	}

	return connect.NewResponse(&organizationv1.ListPluginsResponse{
		Plugins: adapter.FromPlugins(plugins),
	}), nil
}
