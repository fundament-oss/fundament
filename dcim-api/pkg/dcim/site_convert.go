package dcim

import (
	db "github.com/fundament-oss/fundament/dcim-api/pkg/db/gen"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func siteFromRow(row *db.SiteGetByIDRow) *dcimv1.Site {
	site := dcimv1.Site_builder{
		Id:      row.ID.String(),
		Name:    row.Name,
		Created: timestamppb.New(row.Created.Time),
	}.Build()

	if row.Address.Valid {
		site.SetAddress(row.Address.String)
	}

	return site
}

func siteFromListRow(row *db.SiteListRow) *dcimv1.Site {
	site := dcimv1.Site_builder{
		Id:      row.ID.String(),
		Name:    row.Name,
		Created: timestamppb.New(row.Created.Time),
	}.Build()

	if row.Address.Valid {
		site.SetAddress(row.Address.String)
	}

	return site
}
