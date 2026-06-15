package authn

import (
	"context"
	"errors"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	authnv1 "github.com/fundament-oss/fundament/authn-api/pkg/proto/gen/authn/v1"
	"github.com/fundament-oss/fundament/common/auth"
	"github.com/fundament-oss/fundament/common/authz"
)

// PluginTokenExpiry is the TTL of a minted PluginToken. Plugins re-mint
// rather than refresh; the short window is what bounds the revocation lag.
// See FUN-17 "Two token types".
const PluginTokenExpiry = 15 * time.Minute

// MintPluginToken issues a short-lived PluginToken (aud=fundament-plugin)
// for a user-acting-through-installation. Caller authenticates with a
// UserToken. The minted token carries identity (user) and binding
// (cluster, installation) but no scope: scope is read live from the
// PluginInstallation CR by kube-api-proxy. See FUN-17.
func (s *AuthnServer) MintPluginToken(
	ctx context.Context,
	req *connect.Request[authnv1.MintPluginTokenRequest],
) (*connect.Response[authnv1.MintPluginTokenResponse], error) {
	claims, err := s.validator.Validate(req.Header())
	if err != nil {
		s.logger.Debug("mint plugin token: user token validation failed", "error", err)
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("invalid user token"))
	}

	clusterID := uuid.MustParse(req.Msg.GetClusterId())
	installationID := uuid.MustParse(req.Msg.GetInstallationId())

	userID := claims.UserID()
	manifest, err := s.resolveInstallation(ctx, userID, clusterID, installationID)
	if err != nil {
		return nil, err
	}

	accessToken, err := s.signPluginToken(userID, clusterID, installationID, manifest)
	if err != nil {
		s.logger.Error("mint plugin token: signing failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("internal error"))
	}

	s.logger.Info("plugin token minted",
		"user_id", userID,
		"cluster_id", clusterID,
		"installation_id", installationID,
		"plugin_name", manifest.PluginName,
		"plugin_version", manifest.PluginVersion,
	)

	return connect.NewResponse(authnv1.MintPluginTokenResponse_builder{
		AccessToken: accessToken,
		TokenType:   "Bearer",
		ExpiresIn:   int64(PluginTokenExpiry.Seconds()),
	}.Build()), nil
}

// resolveInstallation runs the two-gate authorization check: OpenFGA
// can_view on the cluster, then a plugin-proxy installation lookup.
func (s *AuthnServer) resolveInstallation(
	ctx context.Context,
	userID, clusterID, installationID uuid.UUID,
) (*InstallationManifest, error) {
	decision, err := s.authz.Evaluate(ctx, authz.EvaluationRequest{
		Subject:  authz.User(userID),
		Action:   authz.CanView(),
		Resource: authz.Cluster(clusterID),
	})
	if err != nil {
		s.logger.Error("mint plugin token: openfga evaluation failed",
			"error", err, "user_id", userID, "cluster_id", clusterID)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("internal error"))
	}
	if !decision.Decision {
		s.logger.Debug("mint plugin token: cluster view denied",
			"user_id", userID, "cluster_id", clusterID)
		// FUN-12: collapse unauthorized + missing to NotFound to avoid
		// leaking existence of resources the caller cannot see.
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("plugin installation not found"))
	}

	manifest, err := s.pluginInstallations.GetInstallationManifest(ctx, clusterID, installationID)
	switch {
	case errors.Is(err, ErrInstallationNotFound):
		s.logger.Debug("mint plugin token: installation not found",
			"cluster_id", clusterID, "installation_id", installationID)
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("plugin installation not found"))
	case errors.Is(err, ErrInstallationTerminating):
		s.logger.Debug("mint plugin token: installation terminating",
			"cluster_id", clusterID, "installation_id", installationID)
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("plugin installation is terminating"))
	case err != nil:
		s.logger.Error("mint plugin token: installation lookup failed",
			"error", err, "cluster_id", clusterID, "installation_id", installationID)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("internal error"))
	}

	return manifest, nil
}

// signPluginToken produces an HS256-signed PluginToken carrying identity
// (sub=user), binding (cluster_id, installation_id), and audit fields
// (plugin_name, plugin_version, definition_hash). No scope is embedded — see
// FUN-17 "Where the scope comes from".
func (s *AuthnServer) signPluginToken(
	userID, clusterID, installationID uuid.UUID,
	manifest *InstallationManifest,
) (string, error) {
	now := time.Now()
	claims := auth.PluginClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    auth.ConsoleIssuer,
			Subject:   userID.String(),
			Audience:  jwt.ClaimStrings{string(auth.TokenTypePlugin)},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(PluginTokenExpiry)),
		},
		ClusterID:      clusterID.String(),
		InstallationID: installationID.String(),
		PluginName:     manifest.PluginName,
		PluginVersion:  manifest.PluginVersion,
		DefinitionHash: manifest.DefinitionHash,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(s.config.JWTSecret)
	if err != nil {
		return "", fmt.Errorf("signing plugin token: %w", err)
	}
	return signed, nil
}
