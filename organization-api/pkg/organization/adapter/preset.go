package adapter

import (
	"github.com/google/uuid"

	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func FromPresets(presets []db.ZappstorePreset, presetPlugins []db.ZappstorePresetPlugin) []*organizationv1.Preset {
	// Build a map of preset ID to plugin IDs
	pluginsByPreset := make(map[uuid.UUID][]string)
	for _, pp := range presetPlugins {
		pluginsByPreset[pp.PresetID] = append(pluginsByPreset[pp.PresetID], pp.PluginID.String())
	}

	result := make([]*organizationv1.Preset, 0, len(presets))
	for i := range presets {
		result = append(result, FromPreset(&presets[i], pluginsByPreset[presets[i].ID]))
	}
	return result
}

func FromPreset(p *db.ZappstorePreset, pluginIDs []string) *organizationv1.Preset {
	description := ""
	if p.Description.Valid {
		description = p.Description.String
	}

	return &organizationv1.Preset{
		Id:          p.ID.String(),
		Name:        p.Name,
		Description: description,
		PluginIds:   pluginIDs,
	}
}
