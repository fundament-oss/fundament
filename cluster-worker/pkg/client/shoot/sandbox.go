package shoot

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// SandboxShootAccess is the mock shoot access with one exception: namespaces
// live on a real cluster, the local plugin sandbox. Local development has no
// shoots, and kube-api-proxy sends every cluster id to that sandbox, so a
// plugin creating something in a project namespace needs the namespace to exist
// there. Everything else (ServiceAccounts, RBAC, LimitRanges, CRDs, Deployments)
// stays in memory: the sandbox runs its own plugin-controller and grants the
// console users' ServiceAccounts cluster-admin already.
//
// Every cluster id maps to the same sandbox, so two clusters with a project
// namespace of the same name collide: the first one's namespace carries its
// namespace id, and the second's stays pending on the name. One mock cluster
// per name is all local development needs.
type SandboxShootAccess struct {
	*MockShootAccess
	sandbox *RealShootAccess
}

// NewSandboxShootAccess builds a SandboxShootAccess that keeps namespaces on
// the cluster the kubeconfig points at.
func NewSandboxShootAccess(kubeconfig []byte, logger *slog.Logger) (*SandboxShootAccess, error) {
	cfg, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("parse sandbox kubeconfig: %w", err)
	}
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("create sandbox clientset: %w", err)
	}

	sandbox := &RealShootAccess{
		logger: logger.With("component", "sandbox-shoot-access"),
		newClient: func(context.Context, uuid.UUID) (kubernetes.Interface, error) {
			return cs, nil
		},
		newAPIExtClient: func(context.Context, uuid.UUID) (apiextensionsclientset.Interface, error) {
			return nil, fmt.Errorf("sandbox shoot access keeps CRDs in memory")
		},
	}

	return &SandboxShootAccess{
		MockShootAccess: NewMockShootAccess(logger),
		sandbox:         sandbox,
	}, nil
}

func (s *SandboxShootAccess) EnsureNamespace(ctx context.Context, clusterID uuid.UUID, name string) error {
	return s.sandbox.EnsureNamespace(ctx, clusterID, name)
}

func (s *SandboxShootAccess) GetNamespace(ctx context.Context, clusterID uuid.UUID, name string) (*ResourceInfo, error) {
	return s.sandbox.GetNamespace(ctx, clusterID, name)
}

func (s *SandboxShootAccess) CreateNamespace(ctx context.Context, clusterID uuid.UUID, name string, labels map[string]string) error {
	return s.sandbox.CreateNamespace(ctx, clusterID, name, labels)
}

func (s *SandboxShootAccess) UpdateNamespaceLabels(ctx context.Context, clusterID uuid.UUID, name string, labels map[string]string) error {
	return s.sandbox.UpdateNamespaceLabels(ctx, clusterID, name, labels)
}

func (s *SandboxShootAccess) DeleteNamespace(ctx context.Context, clusterID uuid.UUID, name string) error {
	return s.sandbox.DeleteNamespace(ctx, clusterID, name)
}

func (s *SandboxShootAccess) ListNamespaces(ctx context.Context, clusterID uuid.UUID, labelKey string) ([]ResourceInfo, error) {
	return s.sandbox.ListNamespaces(ctx, clusterID, labelKey)
}

func (s *SandboxShootAccess) FindNamespaceByLabel(ctx context.Context, clusterID uuid.UUID, key, value string) (*ResourceInfo, error) {
	return s.sandbox.FindNamespaceByLabel(ctx, clusterID, key, value)
}
