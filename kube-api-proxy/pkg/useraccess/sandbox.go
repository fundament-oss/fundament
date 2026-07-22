package useraccess

import (
	"context"
	"fmt"

	authorizationv1 "k8s.io/api/authorization/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/fundament-oss/fundament/kube-api-proxy/pkg/gardener"
)

// NewSandboxClient returns a Client that issues SubjectAccessReviews directly
// against a kubeconfig loaded from disk. Every clusterID is routed to the same
// cluster (the local plugin sandbox), so the FUN-17 "user half" runs against
// the sandbox apiserver exactly as the Gardener path runs against a shoot —
// wrapping it in [New] yields a real [Checker] with no allow-all shortcut.
//
// The sandbox apiserver evaluates the per-user SA identity
// (system:serviceaccount:fundament-system:fundament-{userID}) against its own
// RBAC. cluster-worker (which provisions those SAs on real clusters) is
// disabled in the sandbox, so `just plugin-sandbox-kubeconfig` grants the
// fundament-system SA group access there — otherwise every request is denied.
func NewSandboxClient(kubeconfigPath string) (Client, error) {
	restConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("load sandbox kubeconfig %q: %w", kubeconfigPath, err)
	}
	cs, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("create sandbox kubernetes client: %w", err)
	}
	return &sandboxClient{cs: cs}, nil
}

type sandboxClient struct {
	cs kubernetes.Interface
}

// SubjectAccessReview ignores clusterID — every cluster is the sandbox — and
// delegates to the shared SAR helper so the sandbox matches the production path
// exactly.
func (s *sandboxClient) SubjectAccessReview(ctx context.Context, _ string, user string, groups []string, attrs authorizationv1.ResourceAttributes) (bool, error) {
	return gardener.SubjectAccessReviewWithClientset(ctx, s.cs, user, groups, attrs)
}
