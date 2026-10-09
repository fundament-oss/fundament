package organization

import organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"

// The platform's suggested per-container defaults, offered on the cluster page
// as a starting point. They are suggestions, not floors: a cluster that leaves
// a field unset applies no default for it. Update here if the platform's
// intended values change.
const (
	suggestedMemoryRequestMi = 256
	suggestedMemoryLimitMi   = 512
	suggestedCPURequestM     = 100
	suggestedCPULimitM       = 500
)

func suggestedContainerDefaults() *organizationv1.ContainerDefaults {
	defaults := organizationv1.ContainerDefaults_builder{}.Build()
	defaults.SetMemoryRequestMi(suggestedMemoryRequestMi)
	defaults.SetMemoryLimitMi(suggestedMemoryLimitMi)
	defaults.SetCpuRequestM(suggestedCPURequestM)
	defaults.SetCpuLimitM(suggestedCPULimitM)
	return defaults
}
