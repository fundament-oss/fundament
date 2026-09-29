package install

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/common/auth"
)

var testSecret = []byte("install-test-secret")

func newAuthServer() *Server {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return &Server{
		logger:        logger,
		jwtSecret:     testSecret,
		userValidator: auth.NewValidatorForAudience(testSecret, auth.ConsoleAuthCookieName, auth.ConsoleIssuer, auth.TokenTypeUser, logger),
	}
}

func signWorkload(t *testing.T, orgID uuid.UUID, workload string) string {
	t.Helper()
	claims := auth.WorkloadClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    auth.ConsoleIssuer,
			Subject:   uuid.NewString(),
			Audience:  jwt.ClaimStrings{auth.TokenTypeWorkload},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ID:        uuid.NewString(),
		},
		OrganizationID: orgID.String(),
		Workload:       workload,
	}
	s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(testSecret)
	require.NoError(t, err)
	return s
}

func signUser(t *testing.T, secret []byte, orgIDs ...uuid.UUID) string {
	t.Helper()
	claims := auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    auth.ConsoleIssuer,
			Subject:   uuid.NewString(),
			Audience:  jwt.ClaimStrings{auth.TokenTypeUser},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
		OrganizationIDs: orgIDs,
	}
	s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)
	require.NoError(t, err)
	return s
}

func signPlugin(t *testing.T) string {
	t.Helper()
	claims := auth.PluginClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    auth.ConsoleIssuer,
			Subject:   uuid.NewString(),
			Audience:  jwt.ClaimStrings{auth.TokenTypePlugin},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
		ClusterID:      uuid.NewString(),
		InstallationID: uuid.NewString(),
	}
	s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(testSecret)
	require.NoError(t, err)
	return s
}

func headers(kv ...string) http.Header {
	h := http.Header{}
	for i := 0; i+1 < len(kv); i += 2 {
		h.Set(kv[i], kv[i+1])
	}
	return h
}

func TestAuthenticate_WorkloadToken(t *testing.T) {
	s := newAuthServer()
	orgID := uuid.New()

	ctx, err := s.authenticate(context.Background(), "/install.v1.InstallService/GetPluginDefinition",
		headers("Authorization", "Bearer "+signWorkload(t, orgID, auth.WorkloadPluginController)))
	require.NoError(t, err)
	got, ok := OrganizationIDFromContext(ctx)
	require.True(t, ok)
	assert.Equal(t, orgID, got, "the organization is the token's claim")
}

func TestAuthenticate_UserTokenWithOrganizationHeader(t *testing.T) {
	s := newAuthServer()
	orgID := uuid.New()

	ctx, err := s.authenticate(context.Background(), "/install.v1.InstallService/ListPlugins",
		headers("Authorization", "Bearer "+signUser(t, testSecret, uuid.New(), orgID), OrganizationHeader, orgID.String()))
	require.NoError(t, err)
	got, ok := OrganizationIDFromContext(ctx)
	require.True(t, ok)
	assert.Equal(t, orgID, got)
}

func TestAuthenticate_UserCookie(t *testing.T) {
	s := newAuthServer()
	orgID := uuid.New()

	ctx, err := s.authenticate(context.Background(), "/install.v1.InstallService/ListPlugins",
		headers("Cookie", auth.ConsoleAuthCookieName+"="+signUser(t, testSecret, orgID), OrganizationHeader, orgID.String()))
	require.NoError(t, err)
	_, ok := OrganizationIDFromContext(ctx)
	assert.True(t, ok, "the console's cookie session is a UserToken too")
}

func TestAuthenticate_Rejects(t *testing.T) {
	s := newAuthServer()
	orgID := uuid.New()
	tests := []struct {
		name   string
		header http.Header
		code   connect.Code
	}{
		{"no credential", headers(OrganizationHeader, orgID.String()), connect.CodeUnauthenticated},
		{"garbage bearer", headers("Authorization", "Bearer nope"), connect.CodeUnauthenticated},
		{"plugin token", headers("Authorization", "Bearer "+signPlugin(t), OrganizationHeader, orgID.String()), connect.CodeUnauthenticated},
		{"workload token wrong secret", headers("Authorization", "Bearer "+func() string {
			claims := auth.WorkloadClaims{RegisteredClaims: jwt.RegisteredClaims{
				Issuer: auth.ConsoleIssuer, Subject: uuid.NewString(), Audience: jwt.ClaimStrings{auth.TokenTypeWorkload},
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
			}, OrganizationID: orgID.String(), Workload: auth.WorkloadPluginController}
			tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("other"))
			require.NoError(t, err)
			return tok
		}()), connect.CodeUnauthenticated},
		{"workload token of another workload", headers("Authorization", "Bearer "+signWorkload(t, orgID, "metrics-agent")), connect.CodePermissionDenied},
		{"workload token with organization header", headers("Authorization", "Bearer "+signWorkload(t, orgID, auth.WorkloadPluginController), OrganizationHeader, orgID.String()), connect.CodeInvalidArgument},
		{"user token without organization header", headers("Authorization", "Bearer "+signUser(t, testSecret, orgID)), connect.CodeInvalidArgument},
		{"user token with malformed organization header", headers("Authorization", "Bearer "+signUser(t, testSecret, orgID), OrganizationHeader, "acme"), connect.CodeInvalidArgument},
		{"user token for an organization the user is not in", headers("Authorization", "Bearer "+signUser(t, testSecret, uuid.New()), OrganizationHeader, orgID.String()), connect.CodePermissionDenied},
		{"user token wrong secret", headers("Authorization", "Bearer "+signUser(t, []byte("other"), orgID), OrganizationHeader, orgID.String()), connect.CodeUnauthenticated},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := s.authenticate(context.Background(), "/install.v1.InstallService/ListPlugins", tt.header)
			require.Error(t, err)
			assert.Equal(t, tt.code, connect.CodeOf(err), "err: %v", err)
		})
	}
}
