package gardener

import (
	"log/slog"
	"testing"
	"time"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
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

// EnforceShootSettings disables shoot-dns-service on managed Shoots that have
// it on, and writes neither a Shoot that already complies, nor one being
// deleted, nor one this worker does not manage.
func TestEnforceShootSettings(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, gardencorev1beta1.AddToScheme(scheme))

	managed := map[string]string{LabelClusterID: uuid.New().String()}
	enabled := listShootsTestShoot("enabled", managed)
	enabled.Spec.Extensions = []gardencorev1beta1.Extension{{Type: "shoot-dns-service"}}
	compliant := listShootsTestShoot("compliant", managed)
	compliant.Spec.Extensions = []gardencorev1beta1.Extension{{Type: "shoot-dns-service", Disabled: new(true)}}
	deleting := listShootsTestShoot("deleting", managed)
	deleting.DeletionTimestamp = &metav1.Time{Time: time.Now()}
	deleting.Finalizers = []string{"gardener"}
	foreign := listShootsTestShoot("foreign", nil)

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(enabled, compliant, deleting, foreign).Build()
	r := &RealClient{
		client:   c,
		provider: ProviderConfig{Type: "metal"},
		logger:   slog.Default(),
	}

	get := func(name string) *gardencorev1beta1.Shoot {
		shoot := &gardencorev1beta1.Shoot{}
		require.NoError(t, c.Get(t.Context(), client.ObjectKey{Namespace: enabled.Namespace, Name: name}, shoot))
		return shoot
	}
	compliantVersion := get("compliant").ResourceVersion

	require.NoError(t, r.EnforceShootSettings(t.Context()))

	require.Len(t, get("enabled").Spec.Extensions, 1)
	requireTenantDNSDisabled(t, &get("enabled").Spec.Extensions[0])
	require.Equal(t, compliantVersion, get("compliant").ResourceVersion)
	require.Empty(t, get("deleting").Spec.Extensions)
	require.Empty(t, get("foreign").Spec.Extensions)
}

// The local provider's Shoots are left alone.
func TestEnforceShootSettings_LocalProvider(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, gardencorev1beta1.AddToScheme(scheme))

	shoot := listShootsTestShoot("local", map[string]string{LabelClusterID: uuid.New().String()})
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(shoot).Build()
	r := &RealClient{client: c, provider: NewProviderConfig(), logger: slog.Default()}

	require.NoError(t, r.EnforceShootSettings(t.Context()))

	got := &gardencorev1beta1.Shoot{}
	require.NoError(t, c.Get(t.Context(), client.ObjectKeyFromObject(shoot), got))
	require.Empty(t, got.Spec.Extensions)
}
