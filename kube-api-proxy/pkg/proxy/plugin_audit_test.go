package proxy

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/common/auth"
	"github.com/fundament-oss/fundament/kube-api-proxy/pkg/kubereq"
)

func TestAudit_EmitsAttributableLine(t *testing.T) {
	var buf bytes.Buffer
	g := &pluginGateway{logger: slog.New(slog.NewJSONHandler(&buf, nil))}

	claims := &auth.PluginClaims{
		RegisteredClaims: jwt.RegisteredClaims{Subject: uuid.NewString()},
		InstallationID:   uuid.NewString(),
		PluginName:       "cert-manager",
		PluginVersion:    "v1.17.2",
		DefinitionHash:   "sha256:consented",
	}
	attrs := kubereq.Attributes{APIGroup: "cert-manager.io", Resource: "certificates", Verb: "list", Namespace: "team-a"}

	g.audit(claims, &attrs, "sha256:pinned", "allowed")

	var line map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &line), "audit line is not JSON: %s", buf.String())

	for _, key := range []string{
		"user", "installation_id", "plugin_name", "plugin_version",
		"definition_hash", "pinned_definition_hash", "resource", "verb", "decision",
	} {
		assert.Contains(t, line, key, "audit line missing %q; full: %s", key, buf.String())
	}
	assert.Equal(t, "allowed", line["decision"])
	assert.Equal(t, "sha256:pinned", line["pinned_definition_hash"])
}
