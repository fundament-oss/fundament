package catalog

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	catalogv1 "github.com/fundament-oss/fundament/marketplace-api/pkg/proto/gen/catalog/v1"
	marketplacev1 "github.com/fundament-oss/fundament/marketplace-api/pkg/proto/gen/marketplace/v1"
)

// Presets themselves are public curation, but their membership names plugins,
// so preset_plugins_select_catalog gates each row through the plugin's own
// policy: a preset carrying a RESTRICTED or unpublished listing simply comes
// back without it.
func (s *Server) ListPresets(
	ctx context.Context,
	_ *catalogv1.ListPresetsRequest,
) (*catalogv1.ListPresetsResponse, error) {
	rows, err := s.queries.PresetList(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("listing presets: %w", err))
	}

	// One query for all membership rather than one per preset.
	members, err := s.queries.PresetPluginsList(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("listing preset plugins: %w", err))
	}

	pluginsByPreset := map[uuid.UUID][]string{}
	for _, member := range members {
		pluginsByPreset[member.PresetID] = append(pluginsByPreset[member.PresetID], member.PluginID.String())
	}

	presets := make([]*marketplacev1.Preset, 0, len(rows))
	for _, row := range rows {
		presets = append(presets, marketplacev1.Preset_builder{
			Id:          row.ID.String(),
			Name:        row.Name,
			Description: textOrEmpty(row.Description),
			PluginIds:   pluginsByPreset[row.ID],
		}.Build())
	}

	return catalogv1.ListPresetsResponse_builder{
		Presets: presets,
	}.Build(), nil
}
