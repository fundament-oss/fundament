package auth

import (
	"fmt"
	"slices"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// PluginClaims is the parsed shape of a PluginToken (aud=fundament-plugin).
// It carries identity (sub) and binding (cluster, installation) only — scope
// lives on the PluginInstallation CR and is enforced by the cluster.
type PluginClaims struct {
	jwt.RegisteredClaims
	// ClusterID and InstallationID are the hard binding. Proxies reject the
	// token unless the request URL matches both.
	ClusterID      string `json:"cluster_id"`
	InstallationID string `json:"installation_id"`
	// InstallationName is the PluginInstallation CR's metadata.name. It lets
	// kube-api-proxy address the plugin's SA (plugin-{name}) directly, since the
	// CR UID in InstallationID isn't name-addressable via the kube API.
	// InstallationID stays authoritative: the resolver verifies the named CR's
	// UID against it.
	InstallationName string `json:"installation_name"`
	// PluginName/PluginVersion are audit fields; InstallationID is authoritative.
	PluginName    string `json:"plugin_name"`
	PluginVersion string `json:"plugin_version"`
	// DefinitionHash is the content hash of the PluginDefinition the user
	// consented to at mint time. Audit only.
	DefinitionHash string `json:"definition_hash"`
}

// ParsePluginToken parses and verifies a PluginToken with the given HMAC
// secret. It checks signing method, signature, expiry, issuer, that the
// audience contains fundament-plugin, and that the subject is a UUID. It does
// NOT check the cluster/installation binding — that is the caller's job.
func ParsePluginToken(tokenStr string, secret []byte) (*PluginClaims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &PluginClaims{}, func(t *jwt.Token) (any, error) {
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
		return nil, fmt.Errorf("invalid plugin token: %w", err)
	}
	c, ok := token.Claims.(*PluginClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid plugin token claims")
	}
	if !slices.Contains(c.Audience, TokenTypePlugin) {
		return nil, fmt.Errorf("token audience %v does not contain %q", c.Audience, TokenTypePlugin)
	}
	if _, err := uuid.Parse(c.Subject); err != nil {
		return nil, fmt.Errorf("invalid user ID in token subject: %w", err)
	}
	return c, nil
}
