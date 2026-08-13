package shoot

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/gardener"
)

// fakeKubeconfig is the minimum RESTConfigFromKubeConfig accepts; the server is
// never dialled because these tests only build clients, they don't call verbs.
const fakeKubeconfig = `apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://shoot.invalid
  name: shoot
contexts:
- context:
    cluster: shoot
    user: shoot
  name: shoot
current-context: shoot
users:
- name: shoot
  user:
    token: fake
`

// countingGardener answers admin-kubeconfig requests and counts them. The
// embedded nil interface panics on every other method, which is the point: the
// credential path must not reach for anything else.
type countingGardener struct {
	gardener.Client
	calls int
}

func (g *countingGardener) RequestAdminKubeconfig(context.Context, uuid.UUID, int64) (*gardener.AdminKubeconfig, error) {
	g.calls++
	return &gardener.AdminKubeconfig{Kubeconfig: []byte(fakeKubeconfig)}, nil
}

func countingAccess(t *testing.T) (*RealShootAccess, *countingGardener) {
	t.Helper()
	g := &countingGardener{}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	return NewRealShootAccess(g, logger), g
}

// Inside one batch a cluster's credentials are minted once and reused by every
// verb, including the apiextensions client that EnsureCRD needs. Each request
// is a Garden shoot lookup plus a CA-signed certificate, and provisioning walks
// six resources per shoot.
func TestClientCache_OneCredentialSetPerClusterPerBatch(t *testing.T) {
	t.Parallel()
	r, g := countingAccess(t)

	ctx := WithClientCache(context.Background())
	clusterID := uuid.New()

	for range 3 {
		_, err := r.newClient(ctx, clusterID)
		require.NoError(t, err)
	}
	_, err := r.newAPIExtClient(ctx, clusterID)
	require.NoError(t, err)

	assert.Equal(t, 1, g.calls)
}

// The cache is keyed per cluster: one shoot's admin kubeconfig must never be
// handed to another.
func TestClientCache_SeparateCredentialsPerCluster(t *testing.T) {
	t.Parallel()
	r, g := countingAccess(t)

	ctx := WithClientCache(context.Background())

	_, err := r.newClient(ctx, uuid.New())
	require.NoError(t, err)
	_, err = r.newClient(ctx, uuid.New())
	require.NoError(t, err)

	assert.Equal(t, 2, g.calls)
}

// Without the opt-in every call mints its own credentials, so a caller that
// holds a context for longer than a batch cannot end up reusing expired ones.
func TestClientCache_OptInOnly(t *testing.T) {
	t.Parallel()
	r, g := countingAccess(t)

	ctx := context.Background()
	clusterID := uuid.New()

	for range 3 {
		_, err := r.newClient(ctx, clusterID)
		require.NoError(t, err)
	}

	assert.Equal(t, 3, g.calls)
	assert.False(t, HasClientCache(ctx))
}
