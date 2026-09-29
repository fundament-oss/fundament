package shootverify

import (
	"context"
	"fmt"
	"slices"

	"github.com/golang-jwt/jwt/v5"
)

// MockVerifier stands in for a cluster's API server in tests and mock-mode
// deployments (PR environments, the cucumber suite). It accepts HS256 tokens
// signed with a shared mock secret whose aud contains the audience, for one
// cluster id only, and reports the token's sub as the username — so a test
// mints "a projected token" the same way a shoot would, minus the cluster.
type MockVerifier struct {
	secret         []byte
	clusterID      string
	organizationID string
}

// NewMockVerifier returns a MockVerifier for the seeded cluster.
func NewMockVerifier(secret []byte, clusterID, organizationID string) *MockVerifier {
	return &MockVerifier{secret: secret, clusterID: clusterID, organizationID: organizationID}
}

// Verify implements Verifier.
func (m *MockVerifier) Verify(_ context.Context, clusterID, token, audience string) (*Subject, error) {
	if clusterID != m.clusterID {
		return nil, fmt.Errorf("%w: %s is not the mock cluster", ErrUnknownCluster, clusterID)
	}
	claims := &jwt.RegisteredClaims{}
	_, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return m.secret, nil
	},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnauthenticated, err)
	}
	if !slices.Contains(claims.Audience, audience) {
		return nil, fmt.Errorf("%w: audience %q not in %v", ErrUnauthenticated, audience, claims.Audience)
	}
	return &Subject{
		Username:       claims.Subject,
		UID:            "mock",
		CredentialID:   claims.ID,
		OrganizationID: m.organizationID,
	}, nil
}
