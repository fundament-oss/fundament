// Package plugindev implements the `functl plugin dev` inner loop.
package plugindev

// Target is the cluster and registry a dev plugin is deployed to.
type Target struct {
	KubeContext         string
	Registry            string
	ControllerNamespace string
	DocsURL             string
}

// Sandbox is the only supported target, the local k3d plugin sandbox.
var Sandbox = Target{
	KubeContext:         "k3d-fundament-plugin",
	Registry:            "localhost:5112",
	ControllerNamespace: "fundament",
	DocsURL:             "https://github.com/fundament-oss/fundament/blob/master/docs/developer/plugins/testing-plugins-locally.md",
}
