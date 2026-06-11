package authn

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	authnv1 "github.com/fundament-oss/fundament/authn-api/pkg/proto/gen/authn/v1"
	"github.com/fundament-oss/fundament/common/auth"
	"github.com/fundament-oss/fundament/common/authz"
)

type fakeAuthz struct {
	allow bool
	err   error
	calls int
}

// Evaluate matches authzEvaluator. The request is taken by value to match
// the real authz.Client signature; gocritic's hugeParam warning is moot for
// a test fake bound to that interface.
//
//nolint:gocritic // interface match
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
		validator:           auth.NewValidatorForAudience(secret, auth.TokenTypeUser, logger),
		authz:               az,
		pluginInstallations: lk,
	}
	return &mintHarness{
		server:         server,
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
			Subject:   h.userID.String(),
			Audience:  jwt.ClaimStrings{string(auth.TokenTypeUser)},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(h.secret)
	if err != nil {
		t.Fatalf("sign user token: %v", err)
	}
	return signed
}

func (h *mintHarness) request(t *testing.T, authHeader string) *connect.Request[authnv1.MintPluginTokenRequest] {
	t.Helper()
	req := connect.NewRequest(authnv1.MintPluginTokenRequest_builder{
		ClusterId:      h.clusterID.String(),
		InstallationId: h.installationID.String(),
	}.Build())
	if authHeader != "" {
		req.Header().Set("Authorization", authHeader)
	}
	return req
}

func activeManifest() *InstallationManifest {
	return &InstallationManifest{
		PluginName:     "cert-manager",
		PluginVersion:  "1.2.3",
		DefinitionHash: "sha256:1f3c9a",
	}
}

func TestMintPluginToken_Success(t *testing.T) {
	h := newMintHarness(t, true, activeManifest(), nil)
	req := h.request(t, "Bearer "+h.userToken(t))

	resp, err := h.server.MintPluginToken(context.Background(), req)
	if err != nil {
		t.Fatalf("MintPluginToken: %v", err)
	}

	if got := resp.Msg.GetTokenType(); got != "Bearer" {
		t.Errorf("token_type = %q, want Bearer", got)
	}
	if got := resp.Msg.GetExpiresIn(); got != int64(PluginTokenExpiry.Seconds()) {
		t.Errorf("expires_in = %d, want %d", got, int64(PluginTokenExpiry.Seconds()))
	}

	claims, err := auth.ParsePluginToken(resp.Msg.GetAccessToken(), h.secret)
	if err != nil {
		t.Fatalf("parse minted token: %v", err)
	}
	if got := claims.Type(); got != auth.TokenTypePlugin {
		t.Errorf("Type() = %q, want %q", got, auth.TokenTypePlugin)
	}
	if claims.Subject != h.userID.String() {
		t.Errorf("sub = %q, want %q", claims.Subject, h.userID)
	}
	if claims.ClusterID != h.clusterID.String() {
		t.Errorf("cluster_id = %q, want %q", claims.ClusterID, h.clusterID)
	}
	if claims.InstallationID != h.installationID.String() {
		t.Errorf("installation_id = %q, want %q", claims.InstallationID, h.installationID)
	}
	if claims.PluginName != "cert-manager" {
		t.Errorf("plugin_name = %q, want cert-manager", claims.PluginName)
	}
	if claims.PluginVersion != "1.2.3" {
		t.Errorf("plugin_version = %q, want 1.2.3", claims.PluginVersion)
	}
	if claims.DefinitionHash != "sha256:1f3c9a" {
		t.Errorf("definition_hash = %q, want sha256:1f3c9a", claims.DefinitionHash)
	}
}

func TestMintPluginToken_NoAuthorization_Unauthenticated(t *testing.T) {
	h := newMintHarness(t, true, activeManifest(), nil)
	req := h.request(t, "")

	_, err := h.server.MintPluginToken(context.Background(), req)
	assertConnectCode(t, err, connect.CodeUnauthenticated)
}

func TestMintPluginToken_PluginAudienceRejected(t *testing.T) {
	h := newMintHarness(t, true, activeManifest(), nil)
	claims := auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   h.userID.String(),
			Audience:  jwt.ClaimStrings{string(auth.TokenTypePlugin)},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(h.secret)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	req := h.request(t, "Bearer "+signed)

	_, err = h.server.MintPluginToken(context.Background(), req)
	assertConnectCode(t, err, connect.CodeUnauthenticated)
}

func TestMintPluginToken_BadClusterID_InvalidArgument(t *testing.T) {
	h := newMintHarness(t, true, activeManifest(), nil)
	req := connect.NewRequest(authnv1.MintPluginTokenRequest_builder{
		ClusterId:      "not-a-uuid",
		InstallationId: h.installationID.String(),
	}.Build())
	req.Header().Set("Authorization", "Bearer "+h.userToken(t))

	_, err := h.server.MintPluginToken(context.Background(), req)
	assertConnectCode(t, err, connect.CodeInvalidArgument)
}

func TestMintPluginToken_BadInstallationID_InvalidArgument(t *testing.T) {
	h := newMintHarness(t, true, activeManifest(), nil)
	req := connect.NewRequest(authnv1.MintPluginTokenRequest_builder{
		ClusterId:      h.clusterID.String(),
		InstallationId: "not-a-uuid",
	}.Build())
	req.Header().Set("Authorization", "Bearer "+h.userToken(t))

	_, err := h.server.MintPluginToken(context.Background(), req)
	assertConnectCode(t, err, connect.CodeInvalidArgument)
}

func TestMintPluginToken_CanViewDenied_NotFound(t *testing.T) {
	h := newMintHarness(t, false, activeManifest(), nil)
	req := h.request(t, "Bearer "+h.userToken(t))

	_, err := h.server.MintPluginToken(context.Background(), req)
	assertConnectCode(t, err, connect.CodeNotFound)

	if h.lookup.calls != 0 {
		t.Errorf("installation lookup called %d times; expected 0 when cluster view denied", h.lookup.calls)
	}
}

func TestMintPluginToken_InstallationNotFound(t *testing.T) {
	h := newMintHarness(t, true, nil, ErrInstallationNotFound)
	req := h.request(t, "Bearer "+h.userToken(t))

	_, err := h.server.MintPluginToken(context.Background(), req)
	assertConnectCode(t, err, connect.CodeNotFound)
}

func TestMintPluginToken_InstallationTerminating_FailedPrecondition(t *testing.T) {
	h := newMintHarness(t, true, nil, ErrInstallationTerminating)
	req := h.request(t, "Bearer "+h.userToken(t))

	_, err := h.server.MintPluginToken(context.Background(), req)
	assertConnectCode(t, err, connect.CodeFailedPrecondition)
}

func TestMintPluginToken_AuthzError_Internal(t *testing.T) {
	h := newMintHarness(t, true, activeManifest(), nil)
	h.authz.err = errors.New("openfga unreachable")
	req := h.request(t, "Bearer "+h.userToken(t))

	_, err := h.server.MintPluginToken(context.Background(), req)
	assertConnectCode(t, err, connect.CodeInternal)
}

func assertConnectCode(t *testing.T, err error, want connect.Code) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected connect error with code %s, got nil", want)
	}
	var ce *connect.Error
	if !errors.As(err, &ce) {
		t.Fatalf("expected *connect.Error, got %T: %v", err, err)
	}
	if ce.Code() != want {
		t.Fatalf("connect code = %s, want %s (err: %v)", ce.Code(), want, err)
	}
}
