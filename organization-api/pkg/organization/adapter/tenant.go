package adapter

import (
	"time"

	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func FromOrganization(o db.TenantOrganization) *organizationv1.Organization {
	return &organizationv1.Organization{
		Id:      o.ID.String(),
		Name:    o.Name,
		Created: o.Created.Time.Format(time.RFC3339),
	}
}
