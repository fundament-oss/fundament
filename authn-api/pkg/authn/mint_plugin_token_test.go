package authn

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	"connectrpc.com/validate"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authnv1 "github.com/fundament-oss/fundament/authn-api/pkg/proto/gen/authn/v1"
	"github.com/fundament-oss/fundament/authn-api/pkg/proto/gen/authn/v1/authnv1connect"
	"github.com/fundament-oss/fundament/common/auth"
	"github.com/fundament-oss/fundament/common/authz"
)

type fakeAuthz struct {
	allow bool
	err   error
	calls int
}

func (f *fakeAuthz) Evaluate(_ context.Context, _ authz.EvaluationRequest) (authz.Decision, error) {
	f.calls++
	if f.err != nil {
		return authz.Decision{}, f.err
	}
	return authz.Decision{Decision: f.allow}, nil
}

type fakeLookup struct {
	manifest *InstallationManifest
	err      error
	calls    int
}

func (f *fakeLookup) GetInstallationManifest(_ context.Context, _, _ uuid.UUID) (*InstallationManifest, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.manifest, nil
}

const testJWTSecret = "test-secret"

type mintHarness struct {
	server                            *AuthnServer
	client                            authnv1connect.TokenServiceClient
	secret                            []byte
	authz                             *fakeAuthz
	lookup                            *fakeLookup
	clusterID, installationID, userID uuid.UUID
}

func newMintHarness(t *testing.T, allow bool, manifest *InstallationManifest, lookupErr error) *mintHarness {
	t.Helper()
	secret := []byte(testJWTSecret)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	az := &fakeAuthz{allow: allow}
	lk := &fakeLookup{manifest: manifest, err: lookupErr}
	server := &AuthnServer{
		config: &Config{
			JWTSecret:   secret,
			TokenExpiry: 15 * time.Minute,
		},
		logger:              logger,
		validator:           auth.NewValidatorForAudience(secret, auth.ConsoleAuthCookieName, auth.ConsoleIssuer, auth.TokenTypeUser, logger),
		authz:               az,
		pluginInstallations: lk,
	}

	mux := http.NewServeMux()
	path, handler := authnv1connect.NewTokenServiceHandler(server)
	mux.Handle(path, handler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return &mintHarness{
		server:         server,
		client:         authnv1connect.NewTokenServiceClient(srv.Client(), srv.URL),
		secret:         secret,
		authz:          az,
		lookup:         lk,
		clusterID:      uuid.New(),
		installationID: uuid.New(),
		userID:         uuid.New(),
	}
}

func (h *mintHarness) userToken(t *testing.T) string {
	t.Helper()
	claims := auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    auth.ConsoleIssuer,
			Subject:   h.userID.String(),
			Audience:  jwt.ClaimStrings{string(auth.TokenTypeUser)},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(h.secret)
	require.NoError(t, err, "sign user token")
	return signed
}

func (h *mintHarness) mint(t *testing.T, authHeader string) (*authnv1.MintPluginTokenResponse, error) {
	t.Helper()
	ctx, callInfo := connect.NewClientContext(context.Background())
	if authHeader != "" {
		callInfo.RequestHeader().Set("Authorization", authHeader)
	}
	return h.client.MintPluginToken(ctx, authnv1.MintPluginTokenRequest_builder{ //nolint:wrapcheck // tests assert on the raw connect error
		ClusterId:      h.clusterID.String(),
		InstallationId: h.installationID.String(),
	}.Build())
}

const testPluginName = "test-plugin"

func activeManifest() *InstallationManifest {
	return &InstallationManifest{
		InstallationName: "test-plugin-install",
		PluginName:       testPluginName,
		PluginVersion:    "1.2.3",
		DefinitionHash:   "sha256:1f3c9a",
	}
}

func TestMintPluginToken_Success(t *testing.T) {
	h := newMintHarness(t, true, activeManifest(), nil)

	resp, err := h.mint(t, "Bearer "+h.userToken(t))
	require.NoError(t, err, "MintPluginToken")

	assert.Equal(t, "Bearer", resp.GetTokenType())
	assert.Equal(t, int64(PluginTokenExpiry.Seconds()), resp.GetExpiresIn())

	claims, err := auth.ParsePluginToken(resp.GetAccessToken(), h.secret)
	require.NoError(t, err, "parse minted token")

	assert.Equal(t, h.userID.String(), claims.Subject)
	assert.Equal(t, h.clusterID.String(), claims.ClusterID)
	assert.Equal(t, h.installationID.String(), claims.InstallationID)
	assert.Equal(t, "test-plugin-install", claims.InstallationName)
	assert.Equal(t, testPluginName, claims.PluginName)
	assert.Equal(t, "1.2.3", claims.PluginVersion)
	assert.Equal(t, "sha256:1f3c9a", claims.DefinitionHash)
}

func TestMintPluginToken_NoAuthorization_Unauthenticated(t *testing.T) {
	h := newMintHarness(t, true, activeManifest(), nil)

	_, err := h.mint(t, "")
	assertConnectCode(t, err, connect.CodeUnauthenticated)
}

func TestMintPluginToken_PluginAudienceRejected(t *testing.T) {
	h := newMintHarness(t, true, activeManifest(), nil)
	claims := auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    auth.ConsoleIssuer,
			Subject:   h.userID.String(),
			Audience:  jwt.ClaimStrings{string(auth.TokenTypePlugin)},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(h.secret)
	require.NoError(t, err, "sign")

	_, err = h.mint(t, "Bearer "+signed)
	assertConnectCode(t, err, connect.CodeUnauthenticated)
}

func TestMintPluginToken_CanViewDenied_NotFound(t *testing.T) {
	h := newMintHarness(t, false, activeManifest(), nil)

	_, err := h.mint(t, "Bearer "+h.userToken(t))
	assertConnectCode(t, err, connect.CodeNotFound)

	assert.Equal(t, 0, h.lookup.calls, "installation lookup should not be called when cluster view denied")
}

func TestMintPluginToken_InstallationNotFound(t *testing.T) {
	h := newMintHarness(t, true, nil, ErrInstallationNotFound)

	_, err := h.mint(t, "Bearer "+h.userToken(t))
	assertConnectCode(t, err, connect.CodeNotFound)
}

// TestMintedTokenRejectedAsUserToken pins the escalation wall: a freshly
// minted PluginToken (same HMAC secret, same issuer as every UserToken) must
// be rejected by a UserToken validator on the audience pin.
func TestMintedTokenRejectedAsUserToken(t *testing.T) {
	h := newMintHarness(t, true, activeManifest(), nil)

	resp, err := h.mint(t, "Bearer "+h.userToken(t))
	require.NoError(t, err, "MintPluginToken")

	userValidator := auth.NewValidatorForAudience(h.secret,
		auth.ConsoleAuthCookieName, auth.ConsoleIssuer, auth.TokenTypeUser, nil)
	header := http.Header{}
	header.Set("Authorization", "Bearer "+resp.GetAccessToken())
	_, err = userValidator.Validate(header)
	require.Error(t, err, "user validator accepted a PluginToken")
	assert.Contains(t, err.Error(), "audience", "expected audience-mismatch error")
}

// TestMintPluginToken_WrongSecret_Unauthenticated guards the signature gate.
func TestMintPluginToken_WrongSecret_Unauthenticated(t *testing.T) {
	h := newMintHarness(t, true, activeManifest(), nil)
	claims := auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    auth.ConsoleIssuer,
			Subject:   h.userID.String(),
			Audience:  jwt.ClaimStrings{string(auth.TokenTypeUser)},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).
		SignedString([]byte("different-secret"))
	require.NoError(t, err, "sign")

	_, err = h.mint(t, "Bearer "+signed)
	assertConnectCode(t, err, connect.CodeUnauthenticated)
}

// TestMintPluginToken_ExpiredToken_Unauthenticated guards the lifetime gate.
func TestMintPluginToken_ExpiredToken_Unauthenticated(t *testing.T) {
	h := newMintHarness(t, true, activeManifest(), nil)
	claims := auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    auth.ConsoleIssuer,
			Subject:   h.userID.String(),
			Audience:  jwt.ClaimStrings{string(auth.TokenTypeUser)},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Minute)),
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(h.secret)
	require.NoError(t, err, "sign")

	_, err = h.mint(t, "Bearer "+signed)
	assertConnectCode(t, err, connect.CodeUnauthenticated)
}

// TestMintPluginToken_MalformedUUID_InvalidArgument locks in the protovalidate
// gate so the handler's uuid.MustParse never sees a non-UUID input.
func TestMintPluginToken_MalformedUUID_InvalidArgument(t *testing.T) {
	h := newMintHarness(t, true, activeManifest(), nil)

	mux := http.NewServeMux()
	path, handler := authnv1connect.NewTokenServiceHandler(h.server,
		connect.WithInterceptors(validate.NewInterceptor()))
	mux.Handle(path, handler)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := authnv1connect.NewTokenServiceClient(srv.Client(), srv.URL)
	ctx, callInfo := connect.NewClientContext(context.Background())
	callInfo.RequestHeader().Set("Authorization", "Bearer "+h.userToken(t))

	_, err := client.MintPluginToken(ctx, authnv1.MintPluginTokenRequest_builder{
		ClusterId:      "not-a-uuid",
		InstallationId: h.installationID.String(),
	}.Build())
	assertConnectCode(t, err, connect.CodeInvalidArgument)

	assert.Equal(t, 0, h.authz.calls, "authz.Evaluate should not be called when proto validation rejects")
	assert.Equal(t, 0, h.lookup.calls, "installation lookup should not be called when proto validation rejects")
}

func TestMintPluginToken_AuthzError_Internal(t *testing.T) {
	h := newMintHarness(t, true, activeManifest(), nil)
	h.authz.err = errors.New("openfga unreachable")

	_, err := h.mint(t, "Bearer "+h.userToken(t))
	assertConnectCode(t, err, connect.CodeInternal)
}

func assertConnectCode(t *testing.T, err error, want connect.Code) {
	t.Helper()
	require.Error(t, err, "expected connect error with code %s", want)
	var ce *connect.Error
	require.ErrorAs(t, err, &ce, "expected *connect.Error")
	require.Equal(t, want, ce.Code(), "connect code mismatch (err: %v)", err)
}
