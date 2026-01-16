package organization

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	"github.com/fundament-oss/fundament/organization-api/pkg/organization/adapter"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *OrganizationServer) ListPresets(
	ctx context.Context,
	req *connect.Request[organizationv1.ListPresetsRequest],
) (*connect.Response[organizationv1.ListPresetsResponse], error) {
	presets, err := s.queries.PresetList(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list presets: %w", err))
	}

	presetPlugins, err := s.queries.PresetPluginsList(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list preset plugins: %w", err))
	}

	return connect.NewResponse(&organizationv1.ListPresetsResponse{
		Presets: adapter.FromPresets(presets, presetPlugins),
	}), nil
}
