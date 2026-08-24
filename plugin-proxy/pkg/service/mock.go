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
	"github.com/google/uuid"
)

const (
	MockPluginName    = "test-plugin"
	MockPluginVersion = "v1.17.2"
	MockPluginHash    = "sha256@mock"
)

// MockClusterID matches the acme-corp cluster seeded in db/testdata so the
// e2e suite can address the mock cluster by ID.
var MockClusterID = uuid.MustParse("019b4000-2000-7000-8000-000000000001")

// MockInstallationID is the UID of the test-plugin installation served by
// the mock cluster client.
var MockInstallationID = uuid.MustParse("00000000-0000-0000-0000-000000000001")

// MockOrganizationID is the organization that owns MockClusterID.
var MockOrganizationID = uuid.MustParse("019b4000-1000-7000-8000-000000000001")

// MockClusterAccess implements ClusterAccess in-memory with one cluster
// hosting one cert-manager PluginInstallation.
type MockClusterAccess struct {
	client   client.Client
	clusters map[uuid.UUID]uuid.UUID
}

// NewMockClusterAccess returns a ClusterAccess serving the seeded mock
// cluster.
func NewMockClusterAccess() *MockClusterAccess {
	scheme := runtime.NewScheme()
	if err := pluginsv1.AddToScheme(scheme); err != nil {
		panic(fmt.Errorf("add plugins scheme: %w", err))
	}
	cr := &pluginsv1.PluginInstallation{
		ObjectMeta: metav1.ObjectMeta{
			Name: MockPluginName,
			UID:  types.UID(MockInstallationID.String()),
		},
		Spec: pluginsv1.PluginInstallationSpec{
			DefinitionRef: pluginsv1.DefinitionRef{
				PluginName:     MockPluginName,
				PluginVersion:  MockPluginVersion,
				DefinitionHash: MockPluginHash,
			},
		},
		Status: pluginsv1.PluginInstallationStatus{Phase: pluginsv1.PluginPhaseRunning},
	}
	return &MockClusterAccess{
		client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(cr).Build(),
		clusters: map[uuid.UUID]uuid.UUID{
			MockClusterID: MockOrganizationID,
		},
	}
}

func (m *MockClusterAccess) ForCluster(_ context.Context, clusterID uuid.UUID) (*ClusterTarget, error) {
	orgID, ok := m.clusters[clusterID]
	if !ok {
		return nil, fmt.Errorf("cluster %q not known to mock", clusterID)
	}
	return &ClusterTarget{Client: m.client, OrganizationID: orgID}, nil
}
