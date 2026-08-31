package kubename

import (
	"strings"

	"k8s.io/apimachinery/pkg/util/validation"
)

// PluginChildName is the constant name of every namespace-scoped child resource a
// plugin owns (ServiceAccount, Service, Deployment, container). The plugin's
// namespace already disambiguates them, and a name-derived alternative would hit
// the 63-char DNS label limit that the installation name may exceed. Exported so
// plugin-controller, plugin-proxy and kube-api-proxy cannot disagree about it.
const PluginChildName = "plugin"

const (
	// pluginNSPrefix leads every plugin namespace, keeping plugin-owned
	// namespaces distinguishable from tenant ones.
	pluginNSPrefix = "plugin-"
	// pluginHashLen is how many hex digest chars disambiguate a truncated name.
	pluginHashLen = 8
	// pluginNameBudget is the readable portion left once the prefix, the
	// separator and the hash have taken their share of the 63-char label.
	pluginNameBudget = validation.DNS1123LabelMaxLength - len(pluginNSPrefix) - 1 - pluginHashLen
)

// PluginNamespace returns the cluster-side namespace for a PluginInstallation.
// The installation name is the plugin's identity (<organization>-<plugin>) and is
// already apiserver-unique, so hashing it preserves that uniqueness when the
// readable form does not fit a DNS-1123 label.
//
// installationName is assumed to have passed the controller's validation, so it
// is already within the DNS-1123 label charset — hence no Sanitize call here,
// which would strip the dashes that keep the truncated prefix readable.
func PluginNamespace(installationName string) string {
	if len(pluginNSPrefix)+len(installationName) <= validation.DNS1123LabelMaxLength {
		return pluginNSPrefix + installationName
	}
	trunc := strings.TrimRight(installationName[:pluginNameBudget], "-")
	return pluginNSPrefix + trunc + "-" + HashHex([]byte(installationName))[:pluginHashLen]
}
