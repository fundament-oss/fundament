// Package shootverify verifies a shoot workload's projected ServiceAccount
// token against the cluster it claims to come from (FUN-22, "The verifier").
//
// A Verifier answers one question: is this token a valid, audience-bound
// credential issued by cluster X, and for whom? It never decides policy —
// the subject allow-list, the organization cross-check and the signing of a
// WorkloadToken live in the exchange handler. Implementations: Gardener
// (TokenReview through the shared admin-kubeconfig cache), SingleCluster
// (TokenReview against the plugin sandbox's kubeconfig, for local dev), and
// Mock (an HMAC-signed stand-in for tests).
package shootverify

import (
	"context"
	"errors"
	"fmt"
	"slices"

	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// CredentialIDExtraKey is the TokenReview user-extra key carrying the
// credential id of the presented token (Kubernetes 1.30+). Logged for audit.
const CredentialIDExtraKey = "authentication.kubernetes.io/credential-id"

var (
	// ErrUnauthenticated means the cluster did not accept the token for the
	// audience: bad signature, expired, wrong audience, pod or ServiceAccount
	// gone. Deny.
	ErrUnauthenticated = errors.New("token not authenticated by cluster")
	// ErrUnknownCluster means the verifier cannot route to the cluster at all
	// (no shoot for the id, or a single-cluster verifier asked about another
	// id). Deny.
	ErrUnknownCluster = errors.New("unknown cluster")
	// ErrUnavailable means the review could not be performed: API error,
	// timeout, or the cluster's circuit is open. The caller treats it as a
	// deny; it is kept distinct so logs and metrics can tell a dead shoot
	// from a bad token.
	ErrUnavailable = errors.New("verifier unavailable")
)

// Subject is what a successful verification establishes.
type Subject struct {
	// Username is the Kubernetes identity, e.g.
	// system:serviceaccount:fundament-system:plugin-controller.
	Username string
	// UID is the ServiceAccount UID.
	UID string
	// CredentialID is the token's credential id (empty on shoots older than
	// Kubernetes 1.30).
	CredentialID string
	// OrganizationID is the organization the verifier knows the cluster to
	// belong to, from a source independent of the token: the Shoot's
	// fundament.io/organization-id label, or the seeded organization for the
	// sandbox and mock. The exchange must compare it with the database row.
	OrganizationID string
}

// Verifier verifies a token against the cluster identified by clusterID.
type Verifier interface {
	// Verify returns the Subject when the cluster authenticates the token for
	// the audience. Any error is a deny; ErrUnauthenticated, ErrUnknownCluster
	// and ErrUnavailable classify it for logging.
	Verify(ctx context.Context, clusterID, token, audience string) (*Subject, error)
}

// ReviewWithClientset posts a TokenReview to cs and maps the result. It is
// shared by every clientset-backed Verifier so that they behave identically:
// authenticated must be true and the audience must be echoed back, or the
// token is refused.
func ReviewWithClientset(ctx context.Context, cs kubernetes.Interface, token, audience string) (*Subject, error) {
	review := &authenticationv1.TokenReview{
		Spec: authenticationv1.TokenReviewSpec{
			Token:     token,
			Audiences: []string{audience},
		},
	}
	result, err := cs.AuthenticationV1().TokenReviews().Create(ctx, review, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("%w: create TokenReview: %w", ErrUnavailable, err)
	}
	if result.Status.Error != "" {
		return nil, fmt.Errorf("%w: %s", ErrUnauthenticated, result.Status.Error)
	}
	if !result.Status.Authenticated {
		return nil, ErrUnauthenticated
	}
	if !containsAudience(result.Status.Audiences, audience) {
		return nil, fmt.Errorf("%w: audience %q not in %v", ErrUnauthenticated, audience, result.Status.Audiences)
	}
	subject := &Subject{
		Username: result.Status.User.Username,
		UID:      result.Status.User.UID,
	}
	if ids := result.Status.User.Extra[CredentialIDExtraKey]; len(ids) > 0 {
		subject.CredentialID = ids[0]
	}
	return subject, nil
}

// newReviewClientset builds a clientset for TokenReviews without client-go's
// client-side rate limiter (a negative QPS disables it). Its waits fail on
// load rather than on the cluster, and Guarded would count them against a
// healthy cluster. cfg is copied: a shared config must keep its own limiter.
func newReviewClientset(cfg *rest.Config) (kubernetes.Interface, error) {
	cfg = rest.CopyConfig(cfg)
	cfg.QPS = -1
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("create kubernetes client: %w", err)
	}
	return cs, nil
}

func containsAudience(audiences []string, want string) bool {
	return slices.Contains(audiences, want)
}
