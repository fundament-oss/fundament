package organization

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
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/common/auth"
)

// TestAuthenticate_RejectsPluginToken exercises the audience-aware validator
// wired into organization-api's auth interceptor and asserts that a token
// minted for fundament-plugin is rejected with Unauthenticated. This is the
// parallel of the authn-api and kube-api-proxy escalation-wall tests and
// closes the third surface called out in FUN-17.
func TestAuthenticate_RejectsPluginToken(t *testing.T) {
	secret := []byte("test-secret")
	s := &Server{
		logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		authValidator: auth.NewValidatorForAudience(secret, auth.ConsoleAuthCookieName, auth.ConsoleIssuer, auth.TokenTypeUser, nil),
	}

	pluginClaims := auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "fundament-authn-api",
			Subject:   uuid.New().String(),
			Audience:  jwt.ClaimStrings{auth.TokenTypePlugin},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
		},
	}
	tokenStr, err := jwt.NewWithClaims(jwt.SigningMethodHS256, pluginClaims).SignedString(secret)
	require.NoError(t, err)

	header := http.Header{}
	header.Set("Authorization", "Bearer "+tokenStr)

	_, err = s.authenticate(context.Background(), "/organization.v1.OrganizationService/ListOrganizations", header)
	require.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
}

// TestAuthenticate_RejectsWorkloadToken extends the wall to the WorkloadToken
// of FUN-22: organization-api never accepts a cluster-workload principal.
func TestAuthenticate_RejectsWorkloadToken(t *testing.T) {
	secret := []byte("test-secret")
	s := &Server{
		logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		authValidator: auth.NewValidatorForAudience(secret, auth.ConsoleAuthCookieName, auth.ConsoleIssuer, auth.TokenTypeUser, nil),
	}

	workloadClaims := auth.WorkloadClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    auth.ConsoleIssuer,
			Subject:   uuid.New().String(),
			Audience:  jwt.ClaimStrings{auth.TokenTypeWorkload},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
		},
		OrganizationID: uuid.New().String(),
		Workload:       "plugin-controller",
	}
	tokenStr, err := jwt.NewWithClaims(jwt.SigningMethodHS256, workloadClaims).SignedString(secret)
	require.NoError(t, err)

	header := http.Header{}
	header.Set("Authorization", "Bearer "+tokenStr)

	_, err = s.authenticate(context.Background(), "/organization.v1.OrganizationService/ListOrganizations", header)
	require.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
}
