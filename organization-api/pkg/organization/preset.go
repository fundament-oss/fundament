package organization

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"

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

	// Build a map of preset ID to plugin IDs
	pluginsByPreset := make(map[uuid.UUID][]string)
	for _, pp := range presetPlugins {
		pluginsByPreset[pp.PresetID] = append(pluginsByPreset[pp.PresetID], pp.PluginID.String())
	}

	result := make([]*organizationv1.Preset, 0, len(presets))
	for i := range presets {
		description := ""
		if presets[i].Description.Valid {
			description = presets[i].Description.String
		}

		result = append(result, &organizationv1.Preset{
			Id:          presets[i].ID.String(),
			Name:        presets[i].Name,
			Description: description,
			PluginIds:   pluginsByPreset[presets[i].ID],
		})
	}

	return connect.NewResponse(&organizationv1.ListPresetsResponse{
		Presets: result,
	}), nil
}
