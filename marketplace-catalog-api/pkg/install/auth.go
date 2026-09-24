package install

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"connectrpc.com/connect"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/fundament-oss/fundament/common/auth"
)

// OrganizationHeader names the organization a UserToken caller reads as. It
// is required with a UserToken and rejected with a WorkloadToken, whose
// organization is a claim.
const OrganizationHeader = auth.OrganizationHeader

// authenticate resolves the caller and the organization they read as. Every
// install.v1 RPC is organization-scoped, so there are no public exemptions.
//
// A WorkloadToken (aud=fundament-workload) selects the organization from its
// claim; a UserToken (aud=fundament-user, bearer or cookie) needs the
// Fun-Organization header naming one of the caller's memberships. The JWT
// carries the membership list, so no database round trip.
func (s *Server) authenticate(ctx context.Context, _ string, header http.Header) (context.Context, error) {
	if bearer, ok := bearerToken(header); ok && isWorkloadToken(bearer) {
		return s.authenticateWorkload(ctx, bearer, header)
	}

	claims, err := s.userValidator.Validate(header)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}

	organizationID, err := auth.OrganizationFromHeader(claims, header)
	if err != nil {
		return nil, err //nolint:wrapcheck // already a connect error
	}

	return WithOrganizationID(ctx, organizationID), nil
}

func (s *Server) authenticateWorkload(ctx context.Context, bearer string, header http.Header) (context.Context, error) {
	claims, err := auth.ParseWorkloadToken(bearer, s.jwtSecret)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	// plugin-controller is the only workload install.v1 serves today. The claim
	// is checked rather than trusted: another allow-listed workload would
	// otherwise inherit the installer's read.
	if claims.Workload != auth.WorkloadPluginController {
		return nil, connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("workload %q may not read the install surface", claims.Workload))
	}
	if header.Get(OrganizationHeader) != "" {
		// The organization is a claim of the token; a header could only
		// disagree with it.
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("%s is not accepted with a workload token", OrganizationHeader))
	}
	organizationID, err := uuid.Parse(claims.OrganizationID)
	if err != nil {
		// ParseWorkloadToken already checked this; guard against drift.
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("invalid organization in workload token"))
	}
	return WithOrganizationID(ctx, organizationID), nil
}

func bearerToken(header http.Header) (string, bool) {
	value := header.Get("Authorization")
	token, ok := strings.CutPrefix(value, "Bearer ")
	if !ok || token == "" {
		return "", false
	}
	return token, true
}

// isWorkloadToken peeks at the audience without verifying the signature, only
// to pick the parser; each parser then verifies in full.
func isWorkloadToken(token string) bool {
	var claims jwt.RegisteredClaims
	if _, _, err := jwt.NewParser().ParseUnverified(token, &claims); err != nil {
		return false
	}
	return slices.Contains(claims.Audience, auth.TokenTypeWorkload)
}
