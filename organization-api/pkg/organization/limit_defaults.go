package organization

import organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"

// Platform default limit values shown in the console and applied by the
// "Reset to defaults" action. They are starting values, not enforced floors:
// a limit left unset still means "no limit". Update here if the platform's
// intended defaults change.
const (
	defaultMaxNodesPerCluster     = 10
	defaultMaxNodePoolsPerCluster = 5
	defaultMaxNodesPerNodePool    = 5
	defaultMemoryRequestMi        = 256
	defaultMemoryLimitMi          = 512
	defaultCPURequestM            = 100
	defaultCPULimitM              = 500
)

func organizationLimitDefaults() *organizationv1.OrganizationLimits {
	limits := organizationv1.OrganizationLimits_builder{}.Build()
	limits.SetMaxNodesPerCluster(defaultMaxNodesPerCluster)
	limits.SetMaxNodePoolsPerCluster(defaultMaxNodePoolsPerCluster)
	limits.SetMaxNodesPerNodePool(defaultMaxNodesPerNodePool)
	limits.SetDefaultMemoryRequestMi(defaultMemoryRequestMi)
	limits.SetDefaultMemoryLimitMi(defaultMemoryLimitMi)
	limits.SetDefaultCpuRequestM(defaultCPURequestM)
	limits.SetDefaultCpuLimitM(defaultCPULimitM)
	return limits
}

func projectLimitDefaults() *organizationv1.ProjectLimits {
	limits := organizationv1.ProjectLimits_builder{}.Build()
	limits.SetDefaultMemoryRequestMi(defaultMemoryRequestMi)
	limits.SetDefaultMemoryLimitMi(defaultMemoryLimitMi)
	limits.SetDefaultCpuRequestM(defaultCPURequestM)
	limits.SetDefaultCpuLimitM(defaultCPULimitM)
	return limits
}
