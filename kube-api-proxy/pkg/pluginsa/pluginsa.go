// Package pluginsa implements the FUN-17 "plugin half" — it resolves a plugin
// installation ID against the target cluster to a short-lived TokenRequest
// token for the installation's ServiceAccount, plus the definitionHash pinned
// on the CR at request time.
//
// plugin-controller runs on every tenant cluster, so both the PluginInstallation
// CR and the SA it created (`plugin-{installationName}` in namespace
// `plugin-{installationName}`) live on the same cluster the plugin JS is
// trying to reach.
//
// The gateway forwards the request to the target cluster impersonating the
// plugin SA; the cluster's RBAC on that SA — the ClusterRole plugin-controller
// materialised from the pinned definition — decides what verbs the plugin
// can perform.
package pluginsa

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/fundament-oss/fundament/kube-api-proxy/pkg/gardener"
)

// Token is the resolved plugin SA credential + the definition hash pinned on
// the CR at the time it was issued. The plugin gateway audits both together
// so a forensic query can join by hash.
type Token struct {
	Token                string
	PinnedDefinitionHash string
}

// Resolver answers "given (cluster, installation), what SA token do I forward
// with?" It's the plugin gateway's "plugin half" — the target cluster's RBAC
// on the returned SA is the actual authorization gate.
type Resolver interface {
	Resolve(ctx context.Context, clusterID, installationID, installationName string) (Token, error)
}

// Client is the slice of a Gardener-backed cluster access Real depends on to
// read the PluginInstallation CR and TokenRequest against the plugin SA.
// gardener.Client already satisfies this via ResolvePluginSA. installationID is
// the CR's UID (the value carried in the PluginToken claim).
type Client interface {
	ResolvePluginSA(ctx context.Context, clusterID, installationID, installationName string) (*gardener.PluginSAToken, error)
}

// Stub is the mock-mode implementation: returns a canned token/hash so the
// plugin-token path can be exercised against MockClient (which doesn't check
// the token). NOT suitable for forwarding to a real apiserver — wire
// [NewSandboxClient] + [New] instead when the target is a live cluster.
type Stub struct{}

// Resolve returns a fixed token/hash pair.
func (Stub) Resolve(_ context.Context, _, _, _ string) (Token, error) {
	return Token{Token: "mock-plugin-sa-token", PinnedDefinitionHash: "sha256:mock"}, nil //nolint:gosec // mock token
}

// PluginSA reads the PluginInstallation CR and mints a short-lived TokenRequest
// for its SA against the target cluster.
type PluginSA struct {
	client Client
	logger *slog.Logger
}

// New constructs a Real resolver. logger may be nil (falls back to
// slog.Default).
func New(client Client, logger *slog.Logger) *PluginSA {
	if logger == nil {
		logger = slog.Default()
	}
	return &PluginSA{client: client, logger: logger}
}

// Resolve calls into the Gardener client to fetch a projected token for the
// plugin SA on the target cluster and returns it alongside the pinned
// definition hash from the CR.
func (r *PluginSA) Resolve(ctx context.Context, clusterID, installationID, installationName string) (Token, error) {
	sa, err := r.client.ResolvePluginSA(ctx, clusterID, installationID, installationName)
	if err != nil {
		return Token{}, fmt.Errorf("resolve plugin SA for cluster %s installation %s: %w", clusterID, installationID, err)
	}
	return Token{
		Token:                sa.Token,
		PinnedDefinitionHash: sa.PinnedDefinitionHash,
	}, nil
}
