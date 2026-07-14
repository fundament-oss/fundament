// Package useraccess implements the FUN-17 "user half" — a SubjectAccessReview
// against the target cluster with the caller's per-user SA identity.
//
// cluster-worker provisions ServiceAccounts named `fundament-{userID}` in the
// `fundament-system` namespace on every tenant cluster the user has been
// granted access to (via org or project membership). Admin users get bound to
// cluster-admin; members get the SA without a ClusterRoleBinding so the
// tenant cluster's own RBAC decides what verbs they can invoke.
//
// The plugin gateway calls Real.Check with the verb/resource the plugin is
// trying to reach and the calling user's ID — the API server's SAR machinery
// returns the intersection of that user's granted RBAC and the requested
// action.
package useraccess

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	authorizationv1 "k8s.io/api/authorization/v1"

	"github.com/fundament-oss/fundament/kube-api-proxy/pkg/kubereq"
)

// userSAPrefix is the SAR Spec.User format cluster-worker provisions. See
// cluster-worker/pkg/client/shoot/access.go:12 for the namespace constant and
// cluster-worker/pkg/handler/usersync/mod.go for the SA-name construction.
const userSAPrefix = "system:serviceaccount:fundament-system:fundament-"

// Checker answers "may this user perform this action on this cluster?" It's
// the plugin gateway's "user half": a per-request gate that runs alongside
// OpenFGA can_view and the plugin SA's cluster-side RBAC.
type Checker interface {
	Check(ctx context.Context, clusterID string, attrs *kubereq.Attributes, userID string) (bool, error)
}

// Client is the slice of a Gardener-backed cluster access Real depends on to
// issue a SubjectAccessReview against a target cluster. gardener.Client
// already satisfies this via its SubjectAccessReview method; the interface
// exists so tests can substitute a fake without a live Gardener.
type Client interface {
	SubjectAccessReview(ctx context.Context, clusterID string, user string, attrs authorizationv1.ResourceAttributes) (bool, error)
}

// Stub is the mock-mode Checker: allow-all. Local dev and unit tests that
// only exercise the plugin-token path without a real cluster use this.
type Stub struct{}

// Check unconditionally allows the request. Never returns an error.
func (Stub) Check(_ context.Context, _ string, _ *kubereq.Attributes, _ string) (bool, error) {
	return true, nil
}

// UserAccess issues a SubjectAccessReview against the target cluster with the
// caller's per-user SA identity as Spec.User.
type UserAccess struct {
	client Client
	logger *slog.Logger
}

// New constructs a Real checker. logger may be nil (falls back to
// slog.Default) — accepted so plumbing callers don't have to invent one.
func New(client Client, logger *slog.Logger) *UserAccess {
	if logger == nil {
		logger = slog.Default()
	}
	return &UserAccess{client: client, logger: logger}
}

// Check builds ResourceAttributes from the parsed request attrs and asks the
// target cluster whether the per-user SA is allowed to perform the action.
func (r *UserAccess) Check(ctx context.Context, clusterID string, attrs *kubereq.Attributes, userID string) (bool, error) {
	if attrs == nil {
		return false, errors.New("nil attributes")
	}
	sarAttrs := authorizationv1.ResourceAttributes{
		Namespace:   attrs.Namespace,
		Verb:        attrs.Verb,
		Group:       attrs.APIGroup,
		Resource:    attrs.Resource,
		Subresource: attrs.Subresource,
		Name:        attrs.Name,
	}
	allowed, err := r.client.SubjectAccessReview(ctx, clusterID, userSAPrefix+userID, sarAttrs)
	if err != nil {
		return false, fmt.Errorf("SAR for user %s on cluster %s: %w", userID, clusterID, err)
	}
	return allowed, nil
}
