package authn

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"connectrpc.com/connect"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	db "github.com/fundament-oss/fundament/authn-api/pkg/db/gen"
	authnv1 "github.com/fundament-oss/fundament/authn-api/pkg/proto/gen/authn/v1"
	"github.com/fundament-oss/fundament/authn-api/pkg/shootverify"
	"github.com/fundament-oss/fundament/common/auth"
)

const (
	// WorkloadTokenExpiry is the TTL of a minted WorkloadToken. Workloads
	// re-exchange rather than refresh.
	WorkloadTokenExpiry = 15 * time.Minute

	// MaxWorkloadCredentialLifetime is the longest a presented ServiceAccount
	// token may still be valid for: the projected token's expirationSeconds
	// (3600, see cluster-worker manifests) plus clock skew. A token with a
	// later exp is not a projected token fundament issued and is refused
	// before any remote call.
	MaxWorkloadCredentialLifetime = time.Hour
	workloadCredentialClockSkew   = 5 * time.Minute

	// ShootPluginControllerSubject is the plugin-controller ServiceAccount on
	// every shoot, as rendered by cluster-worker.
	ShootPluginControllerSubject = "system:serviceaccount:fundament-system:plugin-controller"
)

// errWorkloadCredential is the one answer every pre-signing failure gets, so
// an unknown cluster, a bad token and a foreign subject are indistinguishable
// from outside (FUN-12). The specific reason goes to the log.
var errWorkloadCredential = errors.New("invalid workload credential")

// clusterLookup is the slice of *db.Queries the exchange uses, as a seam for
// handler tests.
type clusterLookup interface {
	ClusterGetByID(ctx context.Context, arg db.ClusterGetByIDParams) (db.ClusterGetByIDRow, error)
}

// ExchangeWorkloadToken turns a shoot workload's projected ServiceAccount
// token into a WorkloadToken (FUN-22, "The exchange"). Checks, in order:
// local shape of the credential, the cluster row, the cluster's verdict on
// the token, the subject allow-list, and the organization cross-check.
func (s *AuthnServer) ExchangeWorkloadToken(
	ctx context.Context,
	req *authnv1.ExchangeWorkloadTokenRequest,
) (*authnv1.ExchangeWorkloadTokenResponse, error) {
	callInfo, ok := connect.CallInfoForHandlerContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeInternal, errors.New("missing call info in context"))
	}
	clusterID := uuid.MustParse(req.GetClusterId())
	logger := s.logger.With("cluster_id", clusterID)

	credential, err := extractBearerToken(callInfo.RequestHeader().Get("Authorization"))
	if err != nil {
		logger.Info("workload token exchange denied", "reason", "no bearer credential")
		return nil, connect.NewError(connect.CodeUnauthenticated, errWorkloadCredential)
	}

	// 1. Local pre-checks: nothing remote happens for a credential that could
	//    not be a projected token of ours.
	credentialExp, err := precheckWorkloadCredential(credential, time.Now())
	if err != nil {
		logger.Info("workload token exchange denied", "reason", "credential pre-check", "error", err)
		return nil, connect.NewError(connect.CodeUnauthenticated, errWorkloadCredential)
	}

	// 2. The cluster row is the source of the organization.
	row, err := s.clusters.ClusterGetByID(ctx, db.ClusterGetByIDParams{ID: clusterID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			logger.Info("workload token exchange denied", "reason", "unknown or deleted cluster")
			return nil, connect.NewError(connect.CodeUnauthenticated, errWorkloadCredential)
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			// The caller gave up (or sent a deadline too short for the lookup);
			// nothing is wrong on our side.
			logger.Debug("workload token exchange: caller gone before cluster lookup", "error", err)
			code := connect.CodeCanceled
			if errors.Is(ctxErr, context.DeadlineExceeded) {
				code = connect.CodeDeadlineExceeded
			}
			return nil, connect.NewError(code, ctxErr)
		}
		logger.Error("workload token exchange: cluster lookup failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}
	logger = logger.With("organization_id", row.OrganizationID)

	// 3. That cluster's verdict on the token, for our audience.
	subject, err := s.shootVerifier.Verify(ctx, clusterID.String(), credential, auth.WorkloadCredentialAudience)
	if err != nil {
		level := "denied"
		if errors.Is(err, shootverify.ErrUnavailable) {
			level = "unavailable"
		}
		logger.Warn("workload token exchange "+level, "reason", "verifier", "error", err)
		return nil, connect.NewError(connect.CodeUnauthenticated, errWorkloadCredential)
	}
	logger = logger.With("username", subject.Username, "credential_id", subject.CredentialID)

	// 4. Subject allow-list, keyed by workload name.
	workload, ok := s.workloadSubjects[subject.Username]
	if !ok {
		logger.Warn("workload token exchange denied", "reason", "subject not allow-listed")
		return nil, connect.NewError(connect.CodeUnauthenticated, errWorkloadCredential)
	}

	// 5. The verifier's organization must be the row's. A mismatch is an
	//    inconsistency between the database and the shoot, never served.
	if subject.OrganizationID != row.OrganizationID.String() {
		logger.Error("workload token exchange denied: organization mismatch between cluster row and verifier",
			"verifier_organization_id", subject.OrganizationID)
		return nil, connect.NewError(connect.CodeUnauthenticated, errWorkloadCredential)
	}

	now := time.Now()
	exp := now.Add(WorkloadTokenExpiry)
	if credentialExp.Before(exp) {
		exp = credentialExp
	}
	if !exp.After(now) {
		// The credential expired while it was being reviewed; a token capped
		// to it would be dead on arrival.
		logger.Info("workload token exchange denied", "reason", "credential expired during review")
		return nil, connect.NewError(connect.CodeUnauthenticated, errWorkloadCredential)
	}
	accessToken, err := s.signWorkloadToken(clusterID, row.OrganizationID, workload, now, exp)
	if err != nil {
		logger.Error("workload token exchange: signing failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}

	logger.Info("workload token exchanged", "workload", workload, "expires_in", int64(exp.Sub(now).Seconds()))

	return authnv1.ExchangeWorkloadTokenResponse_builder{
		AccessToken: accessToken,
		TokenType:   "Bearer",
		ExpiresIn:   int64(exp.Sub(now).Seconds()),
	}.Build(), nil
}

// precheckWorkloadCredential checks, without verifying the signature (only
// the cluster can), that the credential is a JWT addressed to authn-api whose
// expiry is neither past nor further ahead than a projected token's can be.
// It returns the credential's expiry so the WorkloadToken can be capped to it.
func precheckWorkloadCredential(credential string, now time.Time) (time.Time, error) {
	claims := &jwt.RegisteredClaims{}
	_, _, err := jwt.NewParser().ParseUnverified(credential, claims)
	if err != nil {
		return time.Time{}, fmt.Errorf("not a JWT: %w", err)
	}
	if !slices.Contains(claims.Audience, auth.WorkloadCredentialAudience) {
		return time.Time{}, fmt.Errorf("audience %v does not contain %q", claims.Audience, auth.WorkloadCredentialAudience)
	}
	if claims.ExpiresAt == nil {
		return time.Time{}, errors.New("no exp claim")
	}
	exp := claims.ExpiresAt.Time
	if !exp.After(now) {
		return time.Time{}, errors.New("credential expired")
	}
	if exp.After(now.Add(MaxWorkloadCredentialLifetime + workloadCredentialClockSkew)) {
		return time.Time{}, fmt.Errorf("exp %s is beyond the projected token lifetime", exp.Format(time.RFC3339))
	}
	return exp, nil
}

func (s *AuthnServer) signWorkloadToken(clusterID, organizationID uuid.UUID, workload string, now, exp time.Time) (string, error) {
	claims := auth.WorkloadClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    auth.ConsoleIssuer,
			Subject:   clusterID.String(),
			Audience:  jwt.ClaimStrings{auth.TokenTypeWorkload},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
			ID:        uuid.NewString(),
		},
		OrganizationID: organizationID.String(),
		Workload:       workload,
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.config.JWTSecret)
	if err != nil {
		return "", fmt.Errorf("signing workload token: %w", err)
	}
	return signed, nil
}

// DefaultWorkloadSubjects is the subject allow-list every deployment starts
// from: the shoot-side plugin-controller ServiceAccount.
func DefaultWorkloadSubjects() map[string]string {
	return map[string]string{ShootPluginControllerSubject: auth.WorkloadPluginController}
}
