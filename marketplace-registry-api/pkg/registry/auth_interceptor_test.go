package registry

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

// TestAuthenticate_RejectsWorkloadToken pins the FUN-22 wall for registry.v1:
// a WorkloadToken carries an organization, but the registry is an
// ownership surface for users and must refuse it on the audience pin, even
// when the caller also sends the organization header.
func TestAuthenticate_RejectsWorkloadToken(t *testing.T) {
	secret := []byte("test-secret")
	s := &Server{
		logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		authValidator: auth.NewValidatorForAudience(secret, auth.ConsoleAuthCookieName, auth.ConsoleIssuer, auth.TokenTypeUser, nil),
	}

	organizationID := uuid.New()
	workloadClaims := auth.WorkloadClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    auth.ConsoleIssuer,
			Subject:   uuid.New().String(),
			Audience:  jwt.ClaimStrings{auth.TokenTypeWorkload},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
		},
		OrganizationID: organizationID.String(),
		Workload:       "plugin-controller",
	}
	tokenStr, err := jwt.NewWithClaims(jwt.SigningMethodHS256, workloadClaims).SignedString(secret)
	require.NoError(t, err)

	header := http.Header{}
	header.Set("Authorization", "Bearer "+tokenStr)
	header.Set(OrganizationHeader, organizationID.String())

	_, err = s.authenticate(context.Background(), "/registry.v1.RegistryService/ListPlugins", header)
	require.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
}
