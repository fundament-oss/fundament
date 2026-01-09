package adapter

import (
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func FromPlugins(plugins []db.TenantPlugin) []*organizationv1.Plugin {
	result := make([]*organizationv1.Plugin, 0, len(plugins))
	for i := range plugins {
		result = append(result, FromPlugin(&plugins[i]))
	}
	return result
}

func FromPlugin(p *db.TenantPlugin) *organizationv1.Plugin {
	return &organizationv1.Plugin{
		Id:   p.ID.String(),
		Name: p.Name,
	}
}
