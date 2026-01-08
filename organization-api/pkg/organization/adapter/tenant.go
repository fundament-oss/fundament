package adapter

import (
	"time"

	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func FromTenant(t db.OrganizationTenant) *organizationv1.Tenant {
	return &organizationv1.Tenant{
		Id:      t.ID.String(),
		Name:    t.Name,
		Created: t.Created.Time.Format(time.RFC3339),
	}
}
