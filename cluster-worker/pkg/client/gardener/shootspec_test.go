package gardener

import (
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func testClient(provider ProviderConfig) *RealClient {
	return &RealClient{provider: provider, logger: slog.Default()}
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
	}
}

// The local provider gets the in-code node CIDR and no provider extension config.
func TestBuildShootSpec_LocalDefaults(t *testing.T) {
	r := testClient(NewProviderConfig())

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

	r := testClient(ProviderConfig{
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
	r := testClient(ProviderConfig{Type: "metal", CloudProfile: "metal", Region: "local"})

	cluster := testCluster()
	cluster.Region = ""

	shoot, err := r.buildShootSpec(cluster)
	require.NoError(t, err)
	require.Equal(t, "local", shoot.Spec.Region)
}
