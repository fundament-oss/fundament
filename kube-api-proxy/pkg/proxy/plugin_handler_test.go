package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/common/auth"
	"github.com/fundament-oss/fundament/kube-api-proxy/pkg/kube"
	"github.com/fundament-oss/fundament/kube-api-proxy/pkg/kubereq"
	"github.com/fundament-oss/fundament/kube-api-proxy/pkg/pluginsa"
	"github.com/fundament-oss/fundament/kube-api-proxy/pkg/useraccess"
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

func (stubPluginSA) Resolve(_ context.Context, _, _, _ string) (pluginsa.Token, error) {
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

// TestPluginGateway_AcceptsClusterIDCasingDifference verifies the cluster
// binding compares parsed UUIDs, so a claim that spells the ID in a different
// case still matches the path ID.
func TestPluginGateway_AcceptsClusterIDCasingDifference(t *testing.T) {
	secret := []byte("s")
	clusterID := uuid.New()
	upper := strings.ToUpper(clusterID.String())

	g := newPluginGateway(t, secret, true, true)
	tok := mintPluginToken(t, secret, uuid.NewString(), upper)

	r := httptest.NewRequestWithContext(context.Background(), "GET", "/api/v1/pods", http.NoBody)
	r.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	g.serve(w, r, clusterID.String())

	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
	require.NotNil(t, g.lastForwarded(), "request was not forwarded despite matching cluster UUIDs")
}

// TestPluginGateway_ForwardsClusterIDContext verifies the gateway forwards with
// kube.ClusterIDContextKey set in canonical form (the multi-cluster proxy needs
// it to resolve the target apiserver).
func TestPluginGateway_ForwardsClusterIDContext(t *testing.T) {
	secret := []byte("s")
	clusterID := uuid.New()
	// Claim spells the ID in upper case; the forwarded key must be canonical.
	upper := strings.ToUpper(clusterID.String())

	g := newPluginGateway(t, secret, true, true)
	tok := mintPluginToken(t, secret, uuid.NewString(), upper)

	r := httptest.NewRequestWithContext(context.Background(), "GET", "/api/v1/pods", http.NoBody)
	r.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	g.serve(w, r, clusterID.String())

	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
	fwd := g.lastForwarded()
	require.NotNil(t, fwd, "request was not forwarded")
	got, ok := fwd.Context().Value(kube.ClusterIDContextKey{}).(string)
	require.True(t, ok, "forwarded request is missing kube.ClusterIDContextKey")
	assert.Equal(t, clusterID.String(), got)
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

// TestPluginGateway_AuditsOnErrorPaths verifies every reachable outcome after
// token parsing emits a decision line, so 5xx/4xx responses still leave a
// forensic trail.
func TestPluginGateway_AuditsOnErrorPaths(t *testing.T) {
	secret := []byte("s")

	newGWWithAudit := func(userSAR useraccess.Checker, pluginSA pluginsa.Resolver, canView ClusterViewChecker) (*pluginGateway, *bytes.Buffer) {
		var buf bytes.Buffer
		return &pluginGateway{
			logger:      slog.New(slog.NewJSONHandler(&buf, nil)),
			jwtSecret:   secret,
			userSAR:     userSAR,
			pluginSA:    pluginSA,
			canView:     canView,
			kubeHandler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }),
		}, &buf
	}
	decisionOf := func(t *testing.T, buf *bytes.Buffer) string {
		t.Helper()
		for raw := range bytes.SplitSeq(bytes.TrimSpace(buf.Bytes()), []byte("\n")) {
			var line map[string]any
			if err := json.Unmarshal(raw, &line); err != nil {
				continue
			}
			if line["audit"] == "plugin_request" {
				return line["decision"].(string)
			}
		}
		t.Fatalf("no plugin_request audit line in buffer: %s", buf.String())
		return ""
	}
	newReq := func(path string) *http.Request {
		return httptest.NewRequestWithContext(context.Background(), "GET", path, http.NoBody)
	}
	allowCanView := func(context.Context, uuid.UUID, uuid.UUID) (bool, error) { return true, nil }
	denyCanView := func(context.Context, uuid.UUID, uuid.UUID) (bool, error) { return false, nil }
	errCanView := func(context.Context, uuid.UUID, uuid.UUID) (bool, error) { return false, errors.New("boom") }

	t.Run("bad cluster id", func(t *testing.T) {
		g, buf := newGWWithAudit(stubUserSAR{true}, stubPluginSA{}, allowCanView)
		tok := mintPluginToken(t, secret, uuid.NewString(), uuid.NewString())
		r := newReq("/api/v1/pods")
		r.Header.Set("Authorization", "Bearer "+tok)
		g.serve(httptest.NewRecorder(), r, "not-a-uuid")
		assert.Equal(t, "error:bad-cluster-id", decisionOf(t, buf))
	})

	t.Run("cluster mismatch", func(t *testing.T) {
		g, buf := newGWWithAudit(stubUserSAR{true}, stubPluginSA{}, allowCanView)
		tok := mintPluginToken(t, secret, uuid.NewString(), uuid.NewString())
		r := newReq("/api/v1/pods")
		r.Header.Set("Authorization", "Bearer "+tok)
		g.serve(httptest.NewRecorder(), r, uuid.NewString())
		assert.Equal(t, "denied:cluster-mismatch", decisionOf(t, buf))
	})

	t.Run("can_view error", func(t *testing.T) {
		clusterID := uuid.NewString()
		g, buf := newGWWithAudit(stubUserSAR{true}, stubPluginSA{}, errCanView)
		tok := mintPluginToken(t, secret, uuid.NewString(), clusterID)
		r := newReq("/api/v1/pods")
		r.Header.Set("Authorization", "Bearer "+tok)
		g.serve(httptest.NewRecorder(), r, clusterID)
		assert.Equal(t, "error:can-view", decisionOf(t, buf))
	})

	t.Run("can_view denied", func(t *testing.T) {
		clusterID := uuid.NewString()
		g, buf := newGWWithAudit(stubUserSAR{true}, stubPluginSA{}, denyCanView)
		tok := mintPluginToken(t, secret, uuid.NewString(), clusterID)
		r := newReq("/api/v1/pods")
		r.Header.Set("Authorization", "Bearer "+tok)
		g.serve(httptest.NewRecorder(), r, clusterID)
		assert.Equal(t, "denied:can-view", decisionOf(t, buf))
	})

	t.Run("unparseable request", func(t *testing.T) {
		clusterID := uuid.NewString()
		g, buf := newGWWithAudit(stubUserSAR{true}, stubPluginSA{}, allowCanView)
		tok := mintPluginToken(t, secret, uuid.NewString(), clusterID)
		r := newReq("/nope")
		r.Header.Set("Authorization", "Bearer "+tok)
		g.serve(httptest.NewRecorder(), r, clusterID)
		assert.Equal(t, "error:unparseable-request", decisionOf(t, buf))
	})

	t.Run("SAR error", func(t *testing.T) {
		clusterID := uuid.NewString()
		g, buf := newGWWithAudit(errUserSAR{}, stubPluginSA{}, allowCanView)
		tok := mintPluginToken(t, secret, uuid.NewString(), clusterID)
		r := newReq("/api/v1/pods")
		r.Header.Set("Authorization", "Bearer "+tok)
		g.serve(httptest.NewRecorder(), r, clusterID)
		assert.Equal(t, "error:user-sar", decisionOf(t, buf))
	})

	t.Run("plugin SA resolve error", func(t *testing.T) {
		clusterID := uuid.NewString()
		g, buf := newGWWithAudit(stubUserSAR{true}, errPluginSA{}, allowCanView)
		tok := mintPluginToken(t, secret, uuid.NewString(), clusterID)
		r := newReq("/api/v1/pods")
		r.Header.Set("Authorization", "Bearer "+tok)
		g.serve(httptest.NewRecorder(), r, clusterID)
		assert.Equal(t, "error:plugin-sa", decisionOf(t, buf))
	})
}

type errUserSAR struct{}

func (errUserSAR) Check(_ context.Context, _ string, _ *kubereq.Attributes, _ string) (bool, error) {
	return false, errors.New("boom")
}

type errPluginSA struct{}

func (errPluginSA) Resolve(_ context.Context, _, _, _ string) (pluginsa.Token, error) {
	return pluginsa.Token{}, errors.New("boom")
}

func mintWorkloadToken(t *testing.T, secret []byte, clusterID string) string {
	t.Helper()
	c := &auth.WorkloadClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   clusterID,
			Issuer:    auth.ConsoleIssuer,
			Audience:  jwt.ClaimStrings{auth.TokenTypeWorkload},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
		},
		OrganizationID: uuid.New().String(),
		Workload:       "plugin-controller",
	}
	s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(secret)
	require.NoError(t, err)
	return s
}

// TestWorkloadTokenRejectedOnBothPaths pins the FUN-22 wall for
// kube-api-proxy: a WorkloadToken whose sub is the very cluster in the path
// is refused by the plugin gateway (not a PluginToken) and by the user path
// (not a UserToken), so a workload can never reach a shoot API through the
// proxy with the identity fundament minted for it.
func TestWorkloadTokenRejectedOnBothPaths(t *testing.T) {
	secret := []byte("s")
	clusterID := uuid.New()
	tok := mintWorkloadToken(t, secret, clusterID.String())

	g := newPluginGateway(t, secret, true, true)
	r := httptest.NewRequestWithContext(context.Background(), "GET", "/api/v1/pods", http.NoBody)
	r.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	g.serve(w, r, clusterID.String())
	assert.Equal(t, http.StatusUnauthorized, w.Code, "plugin gateway accepted a WorkloadToken")

	s := &Server{
		logger:        discardLogger(),
		authValidator: auth.NewValidatorForAudience(secret, auth.ConsoleAuthCookieName, auth.ConsoleIssuer, auth.TokenTypeUser, nil),
	}
	assert.Equal(t, auth.TokenTypeUser, peekTokenType(r), "a non-plugin audience is routed to the user path")
	w = httptest.NewRecorder()
	s.handleUserClusterProxy(w, r, clusterID)
	assert.Equal(t, http.StatusUnauthorized, w.Code, "user path accepted a WorkloadToken")
}
