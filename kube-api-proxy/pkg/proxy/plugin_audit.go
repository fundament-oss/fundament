package proxy

import (
	"github.com/fundament-oss/fundament/common/auth"
	"github.com/fundament-oss/fundament/kube-api-proxy/pkg/kubereq"
)

// audit emits the per-request structured line that recovers user attribution
// for PluginToken requests (FUN-17 "Audit and observability"). Forensic queries
// join this with the cluster audit log. definition_hash is what the user
// consented to at mint; pinned_definition_hash is what actually capped the
// request — they differ only across a re-pin.
//
// a may be nil for outcomes that fire before request parsing (bad cluster id,
// cluster mismatch, can_view denial, etc.); the resource-shaped fields are
// still emitted as empty strings so the log schema stays stable.
func (g *pluginGateway) audit(c *auth.PluginClaims, a *kubereq.Attributes, pinnedDefinitionHash, decision string) {
	if a == nil {
		a = &kubereq.Attributes{}
	}
	g.logger.Info("plugin gateway request",
		"audit", "plugin_request",
		"user", c.Subject,
		"installation_id", c.InstallationID,
		"plugin_name", c.PluginName,
		"plugin_version", c.PluginVersion,
		"definition_hash", c.DefinitionHash,
		"pinned_definition_hash", pinnedDefinitionHash,
		"api_group", a.APIGroup,
		"resource", a.Resource,
		"subresource", a.Subresource,
		"name", a.Name,
		"namespace", a.Namespace,
		"verb", a.Verb,
		"decision", decision,
	)
}
