package gardener

import (
	"log/slog"
	"testing"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
)

func testClient(provider *ProviderConfig) *RealClient {
	return &RealClient{provider: *provider, logger: slog.Default()}
}

func testCluster() *ClusterToSync {
	return &ClusterToSync{
		ID:                uuid.New(),
		OrganizationID:    uuid.New(),
		Name:              "demo",
		ShootName:         "demo",
		Namespace:         "garden-test",
		Region:            "local",
		KubernetesVersion: "1.35.6",
		NodePools:         []NodePool{{Name: "workers", MachineType: "local", AutoscaleMin: 1, AutoscaleMax: 3}},
	}
}

// The local provider gets the in-code node CIDR and no provider extension config.
func TestBuildShootSpec_LocalDefaults(t *testing.T) {
	provider := NewProviderConfig()
	r := testClient(&provider)

	shoot, err := r.buildShootSpec(testCluster())
	require.NoError(t, err)

	require.NotNil(t, shoot.Spec.Networking.Nodes)
	require.Equal(t, "10.0.0.0/16", *shoot.Spec.Networking.Nodes)
	require.Nil(t, shoot.Spec.Networking.Pods)
	require.Nil(t, shoot.Spec.Provider.InfrastructureConfig)
	require.Nil(t, shoot.Spec.Provider.ControlPlaneConfig)
}

// A metal-shaped provider omits the node CIDR (metal IPAM allocates it), sets
// pods/services, stamps the raw extension configs verbatim, and merges the
// operator-supplied annotations.
func TestBuildShootSpec_MetalProvider(t *testing.T) {
	infra := `{"apiVersion":"metal.provider.extensions.gardener.cloud/v1alpha1","kind":"InfrastructureConfig","partitionID":"fire"}`
	cp := `{"apiVersion":"metal.provider.extensions.gardener.cloud/v1alpha1","kind":"ControlPlaneConfig"}`

	r := testClient(&ProviderConfig{
		Type:                 "metal",
		CloudProfile:         "metal",
		Region:               "local",
		PodsCIDR:             "10.240.0.0/16",
		ServicesCIDR:         "10.248.0.0/16",
		InfrastructureConfig: infra,
		ControlPlaneConfig:   cp,
		ShootAnnotations:     map[string]string{"cluster.metal-stack.io/tenant": "fundament"},
	})

	shoot, err := r.buildShootSpec(testCluster())
	require.NoError(t, err)

	// Node CIDR left unset so metal allocates a partition range.
	require.Nil(t, shoot.Spec.Networking.Nodes)
	require.Equal(t, "10.240.0.0/16", *shoot.Spec.Networking.Pods)
	require.Equal(t, "10.248.0.0/16", *shoot.Spec.Networking.Services)

	require.NotNil(t, shoot.Spec.Provider.InfrastructureConfig)
	require.JSONEq(t, infra, string(shoot.Spec.Provider.InfrastructureConfig.Raw))
	require.NotNil(t, shoot.Spec.Provider.ControlPlaneConfig)
	require.JSONEq(t, cp, string(shoot.Spec.Provider.ControlPlaneConfig.Raw))

	require.Equal(t, "fundament", shoot.Annotations["cluster.metal-stack.io/tenant"])
}

// An empty cluster region falls back to the provider default.
func TestBuildShootSpec_RegionFallback(t *testing.T) {
	r := testClient(&ProviderConfig{Type: "metal", CloudProfile: "metal", Region: "local"})

	cluster := testCluster()
	cluster.Region = ""

	shoot, err := r.buildShootSpec(cluster)
	require.NoError(t, err)
	require.Equal(t, "local", shoot.Spec.Region)
}

// A cluster with a catalog region overrides the provider defaults: the shoot
// uses the region's cloud profile and its region-name INSIDE that profile
// (which may differ from the user-facing catalog region name).
func TestBuildShootSpec_CatalogRegion(t *testing.T) {
	r := testClient(&ProviderConfig{Type: "metal", CloudProfile: "env-profile", Region: "env-region"})

	cluster := testCluster()
	cluster.Region = "groenekan-1" // user-facing catalog name
	cluster.CloudProfile = "metal"
	cluster.CloudProfileRegion = "local"

	shoot, err := r.buildShootSpec(cluster)
	require.NoError(t, err)
	require.Equal(t, "metal", shoot.Spec.CloudProfile.Name)
	require.Equal(t, "local", shoot.Spec.Region)
}

// Without catalog fields the provider env defaults still apply (legacy cluster).
func TestBuildShootSpec_LegacyClusterUsesProviderDefaults(t *testing.T) {
	r := testClient(&ProviderConfig{Type: "metal", CloudProfile: "metal", Region: "local"})

	cluster := testCluster()

	shoot, err := r.buildShootSpec(cluster)
	require.NoError(t, err)
	require.Equal(t, "metal", shoot.Spec.CloudProfile.Name)
	require.Equal(t, "local", shoot.Spec.Region)
}

func requireTenantDNSDisabled(t *testing.T, extension *gardencorev1beta1.Extension) {
	t.Helper()
	require.Equal(t, "shoot-dns-service", extension.Type)
	require.NotNil(t, extension.Disabled)
	require.True(t, *extension.Disabled)
	require.Nil(t, extension.ProviderConfig)
}

// A new Shoot has shoot-dns-service disabled, except on the local provider,
// whose Gardener does not register the extension.
func TestBuildShootSpec_DisablesTenantDNS(t *testing.T) {
	local := NewProviderConfig()
	shoot, err := testClient(&local).buildShootSpec(testCluster())
	require.NoError(t, err)
	require.Empty(t, shoot.Spec.Extensions)

	metal := &ProviderConfig{Type: "metal", CloudProfile: "metal", Region: "local"}
	shoot, err = testClient(metal).buildShootSpec(testCluster())
	require.NoError(t, err)
	require.Len(t, shoot.Spec.Extensions, 1)
	requireTenantDNSDisabled(t, &shoot.Spec.Extensions[0])
}

// An existing Shoot whose extension entry was filled in by Gardener gets that
// entries replaced by one disabled entry; other extensions are kept as they
// are, and a second update changes nothing.
func TestUpdateShootSpec_DisablesTenantDNS(t *testing.T) {
	r := testClient(&ProviderConfig{Type: "metal", CloudProfile: "metal", Region: "local"})
	other := gardencorev1beta1.Extension{
		Type:           "other",
		ProviderConfig: &runtime.RawExtension{Raw: []byte(`{"keep":true}`)},
	}
	shoot := &gardencorev1beta1.Shoot{
		Spec: gardencorev1beta1.ShootSpec{
			Extensions: []gardencorev1beta1.Extension{
				{Type: "shoot-dns-service", Disabled: new(true)},
				other,
				{
					Type:           "shoot-dns-service",
					ProviderConfig: &runtime.RawExtension{Raw: []byte(`{"syncProvidersFromShootSpecDNS":true}`)},
				},
			},
		},
	}

	require.NoError(t, r.updateShootSpec(shoot, testCluster()))
	require.NoError(t, r.updateShootSpec(shoot, testCluster()))

	require.Len(t, shoot.Spec.Extensions, 2)
	require.Equal(t, other, shoot.Spec.Extensions[0])
	requireTenantDNSDisabled(t, &shoot.Spec.Extensions[1])
}

// A new Shoot has Gardener's maintenance auto-updates switched off.
func TestBuildShootSpec_DisablesAutoUpdate(t *testing.T) {
	local := NewProviderConfig()
	shoot, err := testClient(&local).buildShootSpec(testCluster())
	require.NoError(t, err)
	requireAutoUpdateDisabled(t, shoot)
}

// A sync switches the auto-updates Gardener defaulted to on off, and keeps the
// rest of the maintenance settings.
func TestUpdateShootSpec_DisablesAutoUpdate(t *testing.T) {
	local := NewProviderConfig()
	window := &gardencorev1beta1.MaintenanceTimeWindow{Begin: "220000+0100", End: "230000+0100"}
	shoot := &gardencorev1beta1.Shoot{
		Spec: gardencorev1beta1.ShootSpec{
			Maintenance: &gardencorev1beta1.Maintenance{
				AutoUpdate: &gardencorev1beta1.MaintenanceAutoUpdate{
					KubernetesVersion:   true,
					MachineImageVersion: new(true),
				},
				TimeWindow: window,
			},
		},
	}

	require.NoError(t, testClient(&local).updateShootSpec(shoot, testCluster()))

	requireAutoUpdateDisabled(t, shoot)
	require.Equal(t, window, shoot.Spec.Maintenance.TimeWindow)
}

// A sync never lowers the Kubernetes version or a worker's machine image, as
// Gardener's maintenance may have moved them past the database's.
func TestUpdateShootSpec_KeepsNewerVersions(t *testing.T) {
	provider := NewProviderConfig()
	r := testClient(&provider)
	cluster := testCluster() // 1.35.6, image 1.0.0

	workers, err := r.buildWorkers(cluster)
	require.NoError(t, err)
	workers[0].Machine.Image.Version = new("1.2.0")
	shoot := &gardencorev1beta1.Shoot{
		Spec: gardencorev1beta1.ShootSpec{
			Kubernetes: gardencorev1beta1.Kubernetes{Version: "1.35.10"},
			Provider:   gardencorev1beta1.Provider{Workers: workers},
		},
	}

	require.NoError(t, r.updateShootSpec(shoot, cluster))
	require.Equal(t, "1.35.10", shoot.Spec.Kubernetes.Version)
	require.Equal(t, "1.2.0", *shoot.Spec.Provider.Workers[0].Machine.Image.Version)

	// An upgrade in the database still goes through.
	cluster.KubernetesVersion = "1.36.6"
	require.NoError(t, r.updateShootSpec(shoot, cluster))
	require.Equal(t, "1.36.6", shoot.Spec.Kubernetes.Version)
}

func TestNewerVersion(t *testing.T) {
	tests := []struct {
		desired, current, want string
	}{
		{"1.35.6", "1.35.10", "1.35.10"},
		{"1.35.10", "1.35.6", "1.35.10"},
		{"1.36.6", "1.35.10", "1.36.6"},
		{"1.35.6", "1.35.6", "1.35.6"},
		{"1.35.6", "", "1.35.6"},
		{"1.35", "1.35.4", "1.35.4"},
		{"1.35.6", "not-a-version", "1.35.6"},
		{"v1.35.6", "1.35.10", "1.35.10"},
		{"1.35.6", "v1.35.10", "v1.35.10"},
	}
	for _, tt := range tests {
		require.Equal(t, tt.want, newerVersion(tt.desired, tt.current), "desired %q, current %q", tt.desired, tt.current)
	}
}

func requireAutoUpdateDisabled(t *testing.T, shoot *gardencorev1beta1.Shoot) {
	t.Helper()
	require.NotNil(t, shoot.Spec.Maintenance)
	require.NotNil(t, shoot.Spec.Maintenance.AutoUpdate)
	require.False(t, shoot.Spec.Maintenance.AutoUpdate.KubernetesVersion)
	require.NotNil(t, shoot.Spec.Maintenance.AutoUpdate.MachineImageVersion)
	require.False(t, *shoot.Spec.Maintenance.AutoUpdate.MachineImageVersion)
}

// keepNewerImages keeps only a higher version of the same image in the worker
// of the same name, and reports what it kept.
func TestKeepNewerImages(t *testing.T) {
	image := func(name, version string) gardencorev1beta1.Machine {
		return gardencorev1beta1.Machine{Image: &gardencorev1beta1.ShootMachineImage{Name: name, Version: new(version)}}
	}
	workers := []gardencorev1beta1.Worker{
		{Name: "older", Machine: image("local", "1.0.0")},
		{Name: "newer", Machine: image("local", "2.0.0")},
		{Name: "other-image", Machine: image("local", "1.0.0")},
		{Name: "new-pool", Machine: image("local", "1.0.0")},
	}
	existing := []gardencorev1beta1.Worker{
		{Name: "older", Machine: image("local", "1.2.0")},
		{Name: "newer", Machine: image("local", "1.0.0")},
		{Name: "other-image", Machine: image("gardenlinux", "9.0.0")},
	}

	kept := keepNewerImages(workers, existing)

	require.Equal(t, []string{"older=1.2.0"}, kept)
	require.Equal(t, "1.2.0", *workers[0].Machine.Image.Version)
	require.Equal(t, "2.0.0", *workers[1].Machine.Image.Version)
	require.Equal(t, "1.0.0", *workers[2].Machine.Image.Version)
	require.Equal(t, "1.0.0", *workers[3].Machine.Image.Version)
}
