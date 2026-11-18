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

// Configured extension types are disabled on new Shoots; without configuration
// the extensions list stays empty so Gardener's defaults apply.
func TestBuildShootSpec_DisabledExtensions(t *testing.T) {
	provider := NewProviderConfig()
	shoot, err := testClient(&provider).buildShootSpec(testCluster())
	require.NoError(t, err)
	require.Empty(t, shoot.Spec.Extensions)

	provider.DisabledExtensions = []string{"shoot-dns-service"}
	shoot, err = testClient(&provider).buildShootSpec(testCluster())
	require.NoError(t, err)
	require.Len(t, shoot.Spec.Extensions, 1)
	require.Equal(t, "shoot-dns-service", shoot.Spec.Extensions[0].Type)
	require.NotNil(t, shoot.Spec.Extensions[0].Disabled)
	require.True(t, *shoot.Spec.Extensions[0].Disabled)
}

// An existing Shoot whose extension entry was filled in by Gardener gets that
// entry replaced by a disabled one; other extensions are kept, and a second
// update changes nothing.
func TestUpdateShootSpec_DisabledExtensions(t *testing.T) {
	r := testClient(&ProviderConfig{
		Type:               "metal",
		CloudProfile:       "metal",
		Region:             "local",
		DisabledExtensions: []string{"shoot-dns-service"},
	})
	shoot := &gardencorev1beta1.Shoot{
		Spec: gardencorev1beta1.ShootSpec{
			Extensions: []gardencorev1beta1.Extension{
				{Type: "other"},
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
	require.Equal(t, "other", shoot.Spec.Extensions[0].Type)
	require.Nil(t, shoot.Spec.Extensions[0].Disabled)
	require.Equal(t, "shoot-dns-service", shoot.Spec.Extensions[1].Type)
	require.NotNil(t, shoot.Spec.Extensions[1].Disabled)
	require.True(t, *shoot.Spec.Extensions[1].Disabled)
	require.Nil(t, shoot.Spec.Extensions[1].ProviderConfig)
}
