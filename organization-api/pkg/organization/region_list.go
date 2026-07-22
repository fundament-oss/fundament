package organization

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

// ListRegions returns the region catalog with per-region offerings (kubernetes
// versions + machine types), text-only: the names are exactly what the create
// endpoints accept. Global catalog data: authenticated like every RPC, but no
// permission check or org filter (same pattern as ListPresets).
func (s *Server) ListRegions(
	ctx context.Context,
	req *connect.Request[organizationv1.ListRegionsRequest],
) (*connect.Response[organizationv1.ListRegionsResponse], error) {
	regions, err := s.queries.RegionList(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list regions: %w", err))
	}

	versions, err := s.queries.RegionKubernetesVersionList(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list region kubernetes versions: %w", err))
	}

	machineTypes, err := s.queries.RegionMachineTypeList(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list region machine types: %w", err))
	}

	versionsByRegion := make(map[uuid.UUID][]string)
	for _, v := range versions {
		versionsByRegion[v.RegionID] = append(versionsByRegion[v.RegionID], v.Version)
	}

	machineTypesByRegion := make(map[uuid.UUID][]*organizationv1.RegionMachineType)
	for _, mt := range machineTypes {
		machineTypesByRegion[mt.RegionID] = append(machineTypesByRegion[mt.RegionID], organizationv1.RegionMachineType_builder{
			Name:   mt.Name,
			Lcpu:   mt.Lcpu,
			Memory: mt.Memory,
		}.Build())
	}

	result := make([]*organizationv1.Region, 0, len(regions))
	for _, r := range regions {
		result = append(result, organizationv1.Region_builder{
			Name:               r.Name,
			KubernetesVersions: versionsByRegion[r.ID],
			MachineTypes:       machineTypesByRegion[r.ID],
		}.Build())
	}

	return connect.NewResponse(organizationv1.ListRegionsResponse_builder{
		Regions: result,
	}.Build()), nil
}
