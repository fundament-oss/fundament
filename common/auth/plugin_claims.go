package auth

import (
	"fmt"
	"slices"

	"github.com/golang-jwt/jwt/v5"
)

// PluginClaims is the parsed shape of a PluginToken (aud=fundament-plugin).
//
// Per FUN-17 a PluginToken carries identity and binding ONLY — there is no
// embedded scope. The plugin half of authorization is a real Kubernetes Role
// materialised by plugin-controller from the pinned PluginDefinition and
// enforced by the cluster; the user half is the gateway's per-request
// SubjectAccessReview. The token is a bound capability, not a scope snapshot.
type PluginClaims struct {
	jwt.RegisteredClaims
	// ClusterID and InstallationID are the hard binding. Proxies reject the
	// token unless the request URL matches both.
	ClusterID      string `json:"cluster_id"`
	InstallationID string `json:"installation_id"`
	// PluginName and PluginVersion are audit/log fields; InstallationID is
	// authoritative.
	PluginName    string `json:"plugin_name"`
	PluginVersion string `json:"plugin_version"`
	// DefinitionHash is the content hash of the PluginDefinition the user
	// consented to at mint time. Carried for audit only — the effective scope
	// is the definition pinned on the CR at request time.
	DefinitionHash string `json:"definition_hash"`
}

// ParsePluginToken parses and verifies a PluginToken with the given HMAC
// secret. It checks the signing method, signature, expiry, and that the
// audience is fundament-plugin. It does NOT check the cluster/installation
// binding — that is the caller's job, against its own request context.
func ParsePluginToken(tokenStr string, secret []byte) (*PluginClaims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &PluginClaims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return secret, nil
	})
	if err != nil {
		return nil, fmt.Errorf("invalid plugin token: %w", err)
	}
	c, ok := token.Claims.(*PluginClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid plugin token claims")
	}
	if !slices.Contains(c.Audience, string(TokenTypePlugin)) {
		return nil, fmt.Errorf("token audience %v does not contain %q", c.Audience, TokenTypePlugin)
	}
	return c, nil
}
