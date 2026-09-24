package auth

import (
	"fmt"
	"slices"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// WorkloadPluginController is the workload name authn-api mints for
// plugin-controller ServiceAccounts and the install surface accepts. One
// symbol for both sides, so neither can be renamed alone.
const WorkloadPluginController = "plugin-controller"

// WorkloadClaims is the parsed shape of a WorkloadToken (aud=fundament-workload).
// It represents a workload running on one cluster: sub is the cluster UUID,
// OrganizationID the owning organization as recorded in the database at
// exchange time, and Workload the name the presenting ServiceAccount was
// allow-listed under (e.g. "plugin-controller"). It carries no user, no
// scope and no permissions — what it may do is decided by the surface that
// accepts it. See FUN-22.
type WorkloadClaims struct {
	jwt.RegisteredClaims
	OrganizationID string `json:"organization_id"`
	Workload       string `json:"workload"`
}

// ClusterID returns the cluster UUID carried in sub. ParseWorkloadToken
// guarantees it parses.
func (c *WorkloadClaims) ClusterID() uuid.UUID {
	return uuid.MustParse(c.Subject)
}

// ParseWorkloadToken parses and verifies a WorkloadToken with the given HMAC
// secret. It checks signing method, signature, expiry, issuer, that the
// audience contains fundament-workload, that the subject is a UUID and that
// organization_id is a UUID and workload is non-empty. It does NOT check
// which workload the caller accepts — that is the caller's job.
//
// A WorkloadToken must never be parsed through Validator: its sub is a
// cluster, not a user, and Validator.UserID() would misread it.
func ParseWorkloadToken(tokenStr string, secret []byte) (*WorkloadClaims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &WorkloadClaims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return secret, nil
	},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithExpirationRequired(),
		jwt.WithIssuer(ConsoleIssuer),
	)
	if err != nil {
		return nil, fmt.Errorf("invalid workload token: %w", err)
	}
	c, ok := token.Claims.(*WorkloadClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid workload token claims")
	}
	if !slices.Contains(c.Audience, TokenTypeWorkload) {
		return nil, fmt.Errorf("token audience %v does not contain %q", c.Audience, TokenTypeWorkload)
	}
	if _, err := uuid.Parse(c.Subject); err != nil {
		return nil, fmt.Errorf("invalid cluster ID in token subject: %w", err)
	}
	if _, err := uuid.Parse(c.OrganizationID); err != nil {
		return nil, fmt.Errorf("invalid organization ID in token: %w", err)
	}
	if c.Workload == "" {
		return nil, fmt.Errorf("token has no workload claim")
	}
	return c, nil
}
