package proxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/common/authz"
	tokenpkg "github.com/fundament-oss/fundament/kube-api-proxy/pkg/token"
)

// The request logger's writer must pass flushes through, or watch events
// through the reverse proxy arrive one event late.
func TestStatusRecorder_PassesFlushThrough(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	w := &statusRecorder{ResponseWriter: rec, status: http.StatusOK}

	_, err := w.Write([]byte(`{"type":"MODIFIED"}`))
	require.NoError(t, err)
	require.NoError(t, http.NewResponseController(w).Flush())
	assert.True(t, rec.Flushed)
}

func TestNewNamespaceLister(t *testing.T) {
	t.Parallel()
	authzClient := &authz.Client{}
	logger := discardLogger()

	assert.Nil(t, newNamespaceLister(&Config{Mode: "mock"}, nil, authzClient, nil, logger), "pure mock")
	assert.Nil(t, newNamespaceLister(&Config{Mode: "mock", PluginSandboxKubeconfig: "k"}, nil, nil, nil, logger), "no authz")

	sandbox := newNamespaceLister(&Config{Mode: "mock", PluginSandboxKubeconfig: "k"}, nil, authzClient, nil, logger)
	require.NotNil(t, sandbox)
	token, err := sandbox.proxyToken(context.Background(), "cluster")
	require.NoError(t, err)
	assert.Empty(t, token, "the sandbox kubeconfig's credentials stand in for the proxy token")

	realMode := newNamespaceLister(&Config{Mode: "real"}, tokenpkg.NewCache(fakeSARequester{token: "t"}, logger), authzClient, nil, logger)
	require.NotNil(t, realMode)
	token, err = realMode.proxyToken(context.Background(), "cluster")
	require.NoError(t, err)
	assert.Equal(t, "t", token)
}
