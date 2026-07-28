package assets

import (
	"context"
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
)

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

const testPluginName = "cert-manager"

var testClusterID = uuid.MustParse("019b4000-2000-7000-8000-000000000001")

type stubFetcher struct{}

func (stubFetcher) Fetch(_ context.Context, clusterID uuid.UUID, pluginName, pluginVersion, assetPath string) ([]byte, string, error) {
	return []byte("body-" + clusterID.String() + pluginName + pluginVersion + assetPath), guessContentType(assetPath), nil
}

var testJWTSecret = []byte("test-secret-for-handler-tests")
var testUserID = uuid.MustParse("019b4000-1000-7000-8000-000000000001")

type stubCanView struct{ allow bool }

func (s stubCanView) CanViewCluster(_ context.Context, _, _ uuid.UUID) (bool, error) {
	return s.allow, nil
}

func newTestHandler() http.Handler {
	return newTestHandlerWithAuth(true)
}

func newTestHandlerWithAuth(allow bool) http.Handler {
	validator := auth.NewValidatorForAudience(
		testJWTSecret,
		auth.ConsoleAuthCookieName,
		auth.ConsoleIssuer,
		auth.TokenTypeUser,
		discardLogger(),
	)
	return NewHandler(stubFetcher{}, &CSPConfig{
		ConnectSrc:     []string{"https://kube-api-proxy.test", "https://plugin-proxy.test"},
		FormAction:     []string{"https://kube-api-proxy.test", "https://plugin-proxy.test"},
		FrameAncestors: []string{"https://console.test"},
	}, validator, func(ctx context.Context, userID, clusterID uuid.UUID) (bool, error) { return allow, nil }, discardLogger())
}

// signTestUserToken mints a UserToken matching the validator's expectations.
func signTestUserToken(t *testing.T) string {
	t.Helper()
	claims := jwt.MapClaims{
		"iss": auth.ConsoleIssuer,
		"sub": testUserID.String(),
		"aud": []string{string(auth.TokenTypeUser)},
		"exp": time.Now().Add(15 * time.Minute).Unix(),
		"iat": time.Now().Unix(),
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(testJWTSecret)
	require.NoError(t, err)
	return signed
}

// withAuthCookie adds the console UserToken cookie to a request.
func withAuthCookie(t *testing.T, r *http.Request) *http.Request {
	r.AddCookie(&http.Cookie{Name: auth.ConsoleAuthCookieName, Value: signTestUserToken(t)})
	return r
}

func testURL(asset string) string {
	return "/clusters/" + testClusterID.String() + "/plugins/" + testPluginName + "/v1/console/" + asset
}

func TestHandler_ServesAssetsWithImmutableCache(t *testing.T) {
	h := newTestHandler()
	r := withAuthCookie(t, httptest.NewRequestWithContext(t.Context(), http.MethodGet, testURL("index.html"), http.NoBody))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	require.Equal(t, http.StatusOK, w.Code, "body = %s", w.Body.String())
	// Cache is private + immutable. Private because auth-gated responses must
	// not be served cross-user by shared caches; immutable because the fetcher
	// verifies the URL version against the installed CR, so the versioned URL
	// is content-stable.
	cc := w.Header().Get("Cache-Control")
	assert.Contains(t, cc, "private")
	assert.Contains(t, cc, "immutable")
	assert.NotContains(t, cc, "public")
	assert.True(t, strings.HasPrefix(w.Header().Get("Content-Type"), "text/html"),
		"Content-Type = %q", w.Header().Get("Content-Type"))
}

// errFetcher returns a fixed error so the handler's error mapping can be tested
// without a live cluster.
type errFetcher struct{ err error }

func (f errFetcher) Fetch(_ context.Context, _ uuid.UUID, _, _, _ string) ([]byte, string, error) {
	return nil, "", f.err
}

func TestHandler_VersionMismatchIs404(t *testing.T) {
	validator := auth.NewValidatorForAudience(
		testJWTSecret, auth.ConsoleAuthCookieName, auth.ConsoleIssuer, auth.TokenTypeUser, discardLogger(),
	)
	for name, fetcher := range map[string]Fetcher{
		"mismatch": errFetcher{err: ErrVersionMismatch},
		"notfound": errFetcher{err: ErrInstallationNotFound},
	} {
		t.Run(name, func(t *testing.T) {
			h := NewHandler(fetcher, &CSPConfig{}, validator,
				func(context.Context, uuid.UUID, uuid.UUID) (bool, error) { return true, nil }, discardLogger())
			r := withAuthCookie(t, httptest.NewRequestWithContext(t.Context(), http.MethodGet, testURL("index.html"), http.NoBody))
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			assert.Equal(t, http.StatusNotFound, w.Code,
				"a version mismatch / missing install must 404, not leak via 502")
		})
	}
}

func TestHandler_SetsStrictCSP(t *testing.T) {
	h := newTestHandler()
	r := withAuthCookie(t, httptest.NewRequestWithContext(t.Context(), http.MethodGet, testURL("index.html"), http.NoBody))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	csp := w.Header().Get("Content-Security-Policy")
	for _, want := range []string{
		"default-src 'self'",
		"script-src 'self'",
		"style-src 'self'",
		"connect-src https://kube-api-proxy.test https://plugin-proxy.test",
		"form-action https://kube-api-proxy.test https://plugin-proxy.test",
		"frame-ancestors https://console.test",
		"base-uri 'none'",
		"object-src 'none'",
	} {
		assert.Contains(t, csp, want)
	}
	// FUN-17 forbids 'unsafe-inline' on the plugin path.
	assert.NotContains(t, csp, "unsafe-inline")
	assert.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
}

func TestHandler_RejectsBadPath(t *testing.T) {
	h := newTestHandler()
	cid := testClusterID.String()
	for _, p := range []string{
		"/clusters/" + cid + "/plugins/cert-manager/v1/console/",              // empty file
		"/clusters/" + cid + "/plugins/cert-manager/v1/console/../etc/passwd", // traversal
		"/clusters/" + cid + "/plugins//v1/console/index.html",                // empty name
		"/clusters/" + cid + "/plugins/cert-manager//console/index.html",      // empty version
		"/clusters/not-a-uuid/plugins/cert-manager/v1/console/index.html",     // bad cluster id
		"/plugins/cert-manager/v1/console/index.html",                         // legacy shape rejected
	} {
		r := withAuthCookie(t, httptest.NewRequestWithContext(t.Context(), http.MethodGet, p, http.NoBody))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		assert.GreaterOrEqual(t, w.Code, 300, "expected non-2xx for %q, got %d", p, w.Code)
	}
}

func TestHandler_OnlyGETandHEAD(t *testing.T) {
	h := newTestHandler()
	r := withAuthCookie(t, httptest.NewRequestWithContext(t.Context(), http.MethodPost, testURL("i.html"), http.NoBody))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestHandler_RejectsMissingCookie(t *testing.T) {
	h := newTestHandler()
	// no cookie
	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, testURL("index.html"), http.NoBody)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	assert.Equal(t, http.StatusNotFound, w.Code, "unauth requests collapse to 404 so the endpoint doesn't leak validity of (cluster, plugin, version)")
}

func TestHandler_RejectsWhenCanViewDenies(t *testing.T) {
	h := newTestHandlerWithAuth(false)
	r := withAuthCookie(t, httptest.NewRequestWithContext(t.Context(), http.MethodGet, testURL("index.html"), http.NoBody))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	assert.Equal(t, http.StatusNotFound, w.Code, "unauthorized collapses to 404 for the same reason")
}
