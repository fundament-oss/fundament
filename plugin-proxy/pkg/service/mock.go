package service

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	pluginsv1 "github.com/fundament-oss/fundament/plugin-controller/pkg/api/v1"
)

// MockClusterID matches the acme-corp cluster seeded in db/testdata so the
// e2e suite can address the mock cluster by ID.
const MockClusterID = "019b4000-2000-7000-8000-000000000001"

// MockInstallationUID is the UID of the cert-manager installation served by
// the mock cluster client.
const MockInstallationUID = "00000000-0000-0000-0000-000000000001"

// MockOrganizationID is the organization that owns MockClusterID.
const MockOrganizationID = "019b4000-1000-7000-8000-000000000001"

// MockClusterAccess implements ClusterAccess in-memory with one cluster
// hosting one cert-manager PluginInstallation.
type MockClusterAccess struct {
	client   client.Client
	clusters map[string]string
}

// NewMockClusterAccess returns a ClusterAccess serving the seeded mock
// cluster.
func NewMockClusterAccess() *MockClusterAccess {
	scheme := runtime.NewScheme()
	_ = pluginsv1.AddToScheme(scheme)
	cr := &pluginsv1.PluginInstallation{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cert-manager",
			UID:  types.UID(MockInstallationUID),
		},
		Spec: pluginsv1.PluginInstallationSpec{
			Image: "ghcr.io/example/cert-manager:v1.17.2",
			DefinitionRef: pluginsv1.DefinitionRef{
				PluginName:     "cert-manager",
				PluginVersion:  "v1.17.2",
				DefinitionHash: "sha256:mock",
			},
		},
		Status: pluginsv1.PluginInstallationStatus{Phase: pluginsv1.PluginPhaseRunning},
	}
	return &MockClusterAccess{
		client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(cr).Build(),
		clusters: map[string]string{
			MockClusterID: MockOrganizationID,
		},
	}
}

func (m *MockClusterAccess) ForCluster(_ context.Context, clusterID string) (*ClusterTarget, error) {
	orgID, ok := m.clusters[clusterID]
	if !ok {
		return nil, fmt.Errorf("cluster %q not known to mock", clusterID)
	}
	return &ClusterTarget{Client: m.client, OrganizationID: orgID}, nil
}
