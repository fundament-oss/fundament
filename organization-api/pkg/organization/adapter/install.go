package adapter

import (
	"google.golang.org/protobuf/types/known/timestamppb"

	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func FromInstalls(installs []db.ZappstoreInstall) []*organizationv1.Install {
	result := make([]*organizationv1.Install, 0, len(installs))
	for i := range installs {
		result = append(result, FromInstall(&installs[i]))
	}
	return result
}

func FromInstall(i *db.ZappstoreInstall) *organizationv1.Install {
	return &organizationv1.Install{
		Id:       i.ID.String(),
		PluginId: i.PluginID.String(),
		CreatedAt: timestamppb.New(i.Created.Time),
	}

}
