package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	pluginsv1 "github.com/fundament-oss/fundament/plugin-controller/pkg/api/v1"
)

// NewSandboxClusterAccess returns a ClusterAccess that resolves EVERY
// clusterID to the same controller-runtime client, built from a kubeconfig
// pointed at a locally-running plugin sandbox cluster (typically
// k3d-fundament-plugin). Local-dev shortcut so authn-api's
// MintPluginToken can look up real PluginInstallations by UID instead of the
// hardcoded fake in MockClusterAccess.
//
// OrganizationID falls back to MockOrganizationID (matches the seeded cluster
// row in db/testdata). In prod a real ClusterAccess resolves per-cluster
// tenancy from the fundament DB.
func NewSandboxClusterAccess(kubeconfigPath string) (*SandboxClusterAccess, error) {
	cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("load sandbox kubeconfig %q: %w", kubeconfigPath, err)
	}

	scheme := runtime.NewScheme()
	if err := pluginsv1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("add plugins scheme: %w", err)
	}

	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("build controller-runtime client: %w", err)
	}
	return &SandboxClusterAccess{client: c}, nil
}

// SandboxClusterAccess routes every cluster lookup to a single kubeconfig-backed
// client — the local plugin sandbox cluster.
type SandboxClusterAccess struct {
	client client.Client
}

func (s *SandboxClusterAccess) ForCluster(_ context.Context, _ uuid.UUID) (*ClusterTarget, error) {
	return &ClusterTarget{Client: s.client, OrganizationID: MockOrganizationID}, nil
}
