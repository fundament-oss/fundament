package proxy

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/common/auth"
	"github.com/fundament-oss/fundament/kube-api-proxy/pkg/kubereq"
	"github.com/fundament-oss/fundament/kube-api-proxy/pkg/pluginsa"
)

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func mintPluginToken(t *testing.T, secret []byte, sub, clusterID string) string {
	t.Helper()
	c := &auth.PluginClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   sub,
			Issuer:    auth.ConsoleIssuer,
			Audience:  jwt.ClaimStrings{auth.TokenTypePlugin},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
		},
		ClusterID:      clusterID,
		InstallationID: uuid.New().String(),
		PluginName:     "cert-manager",
		PluginVersion:  "v1.17.2",
		DefinitionHash: "sha256:mock",
	}
	s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(secret)
	require.NoError(t, err)
	return s
}

type stubUserSAR struct{ allow bool }

func (s stubUserSAR) Check(_ context.Context, _ string, _ *kubereq.Attributes, _ string) (bool, error) {
	return s.allow, nil
}

type stubPluginSA struct{}

func (stubPluginSA) Resolve(_ context.Context, _, _ string) (pluginsa.Token, error) {
	return pluginsa.Token{Token: "plugin-sa-token", PinnedDefinitionHash: "sha256:mock"}, nil
}

type stubCanView struct{ allow bool }

func (s stubCanView) CanViewCluster(_ context.Context, _, _ uuid.UUID) (bool, error) {
	return s.allow, nil
}

func newPluginGateway(t *testing.T, secret []byte, sarAllow, canView bool) *pluginGateway {
	t.Helper()
	var forwarded *http.Request
	kubeHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		forwarded = r
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	g := &pluginGateway{
		logger:      discardLogger(),
		jwtSecret:   secret,
		userSAR:     stubUserSAR{allow: sarAllow},
		pluginSA:    stubPluginSA{},
		canView:     func(ctx context.Context, userID, clusterID uuid.UUID) (bool, error) { return canView, nil },
		kubeHandler: kubeHandler,
	}
	g.lastForwarded = func() *http.Request { return forwarded }
	return g
}

func TestPluginGateway_RejectsWrongCluster(t *testing.T) {
	secret := []byte("s")
	g := newPluginGateway(t, secret, true, true)
	tok := mintPluginToken(t, secret, uuid.NewString(), uuid.NewString())

	r := httptest.NewRequestWithContext(context.Background(), "GET", "/api/v1/pods", http.NoBody)
	r.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	g.serve(w, r, uuid.NewString())
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestPluginGateway_RejectsWhenSARDenies(t *testing.T) {
	secret := []byte("s")
	clusterID := uuid.NewString()
	g := newPluginGateway(t, secret, false, true)
	tok := mintPluginToken(t, secret, uuid.NewString(), clusterID)

	r := httptest.NewRequestWithContext(context.Background(), "GET", "/api/v1/pods", http.NoBody)
	r.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	g.serve(w, r, clusterID)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestPluginGateway_ForwardsWithPluginSAToken(t *testing.T) {
	secret := []byte("s")
	clusterID := uuid.NewString()
	g := newPluginGateway(t, secret, true, true)
	tok := mintPluginToken(t, secret, uuid.NewString(), clusterID)

	r := httptest.NewRequestWithContext(context.Background(), "GET", "/apis/cert-manager.io/v1/namespaces/team-a/certificates", http.NoBody)
	r.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	g.serve(w, r, clusterID)

	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
	fwd := g.lastForwarded()
	require.NotNil(t, fwd, "request was not forwarded")
	// FUN-17: the gateway injects the PLUGIN SA token downstream.
	got := SATokenFrom(fwd.Context())
	assert.Equal(t, "plugin-sa-token", got, "downstream SA token")
}

func TestPluginGateway_RejectsCanViewDenied(t *testing.T) {
	secret := []byte("s")
	clusterID := uuid.NewString()
	g := newPluginGateway(t, secret, true, false)
	tok := mintPluginToken(t, secret, uuid.NewString(), clusterID)

	r := httptest.NewRequestWithContext(context.Background(), "GET", "/api/v1/pods", http.NoBody)
	r.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	g.serve(w, r, clusterID)
	assert.Equal(t, http.StatusForbidden, w.Code)
}
