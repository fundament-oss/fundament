package pluginsa

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/kube-api-proxy/pkg/gardener"
)

// fakeClient returns a canned PluginSAToken.
type fakeClient struct {
	token *gardener.PluginSAToken
	err   error

	lastClusterID    string
	lastInstallation string
	lastName         string
	calls            int
}

func (f *fakeClient) ResolvePluginSA(_ context.Context, clusterID, installationID, installationName string) (*gardener.PluginSAToken, error) {
	f.calls++
	f.lastClusterID = clusterID
	f.lastInstallation = installationID
	f.lastName = installationName
	if f.err != nil {
		return nil, f.err
	}
	return f.token, nil
}

func TestStub_ReturnsFixedToken(t *testing.T) {
	tok, err := Stub{}.Resolve(context.Background(), "any-cluster", "any-install", "any-name")
	require.NoError(t, err)
	assert.Equal(t, "mock-plugin-sa-token", tok.Token)
	assert.Equal(t, "sha256:mock", tok.PinnedDefinitionHash)
}

func TestReal_ReturnsTokenAndHash(t *testing.T) {
	fake := &fakeClient{
		token: &gardener.PluginSAToken{
			Token:                "eyJ.plugin-sa.jwt",
			PinnedDefinitionHash: "sha256:abc123",
		},
	}
	r := New(fake, nil)

	tok, err := r.Resolve(context.Background(), "cluster-abc", "019b4000-2000-7000-8000-000000000009", "cert-manager")
	require.NoError(t, err)
	assert.Equal(t, "eyJ.plugin-sa.jwt", tok.Token)
	assert.Equal(t, "sha256:abc123", tok.PinnedDefinitionHash)
	assert.Equal(t, 1, fake.calls)
	assert.Equal(t, "cluster-abc", fake.lastClusterID)
	assert.Equal(t, "019b4000-2000-7000-8000-000000000009", fake.lastInstallation)
	assert.Equal(t, "cert-manager", fake.lastName)
}

func TestReal_ClientErrorPropagates(t *testing.T) {
	fake := &fakeClient{err: errors.New("SA not found")}
	r := New(fake, nil)

	_, err := r.Resolve(context.Background(), "cluster-abc", "install-uid", "cert-manager")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SA not found")
}
