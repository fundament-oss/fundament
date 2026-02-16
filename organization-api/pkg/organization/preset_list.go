package organization

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *Server) ListPresets(
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

	pluginsByPreset := buildPluginsByPreset(presetPlugins)

	result := make([]*organizationv1.Preset, 0, len(presets))
	for i := range presets {
		result = append(result, presetFromRow(&presets[i], pluginsByPreset))
	}

	return connect.NewResponse(&organizationv1.ListPresetsResponse{
		Presets: result,
	}), nil
}

func buildPluginsByPreset(presetPlugins []db.AppstorePresetPlugin) map[uuid.UUID][]string {
	pluginsByPreset := make(map[uuid.UUID][]string)
	for _, pp := range presetPlugins {
		pluginsByPreset[pp.PresetID] = append(pluginsByPreset[pp.PresetID], pp.PluginID.String())
	}
	return pluginsByPreset
}

func presetFromRow(row *db.AppstorePreset, pluginsByPreset map[uuid.UUID][]string) *organizationv1.Preset {
	description := ""
	if row.Description.Valid {
		description = row.Description.String
	}

	return &organizationv1.Preset{
		Id:          row.ID.String(),
		Name:        row.Name,
		Description: description,
		PluginIds:   pluginsByPreset[row.ID],
	}
}
