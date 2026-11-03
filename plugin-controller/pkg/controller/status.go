package controller

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"connectrpc.com/connect"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	pluginmetadatav1 "github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/metadata/proto/gen/v1"
	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/metadata/proto/gen/v1/pluginmetadatav1connect"

	pluginsv1 "github.com/fundament-oss/fundament/plugin-controller/pkg/api/v1"
)

func pluginServiceURL(installationName string) string {
	// The Service is constant-named inside the plugin's own namespace.
	return fmt.Sprintf("http://%s.%s.svc.cluster.local:8080", childResourceName, pluginNamespace(installationName))
}

type statusPoller struct {
	httpClient connect.HTTPClient
}

func newStatusPoller() *statusPoller {
	return &statusPoller{httpClient: &http.Client{Timeout: 5 * time.Second}}
}

func (s *statusPoller) WithClient(client connect.HTTPClient) *statusPoller {
	s.httpClient = client

	return s
}

func (s *statusPoller) poll(ctx context.Context, cr *pluginsv1.PluginInstallation) pluginsv1.PluginInstallationStatus {
	url := pluginServiceURL(cr.Name)
	client := pluginmetadatav1connect.NewPluginMetadataServiceClient(s.httpClient, url)

	resp, err := client.GetStatus(ctx, &pluginmetadatav1.GetStatusRequest{})
	if err != nil {
		// Before the current spec first came up, an unreachable plugin is one
		// whose pod is still starting. After, it is one that stopped answering,
		// typically a crash loop: reporting that as Deploying would show a broken
		// plugin as one still being installed.
		phase := pluginsv1.PluginPhaseDeploying
		if hasSettled(cr) {
			phase = pluginsv1.PluginPhaseDegraded
		}
		return pluginsv1.PluginInstallationStatus{
			Phase:              phase,
			Message:            fmt.Sprintf("plugin not reachable: %v", err),
			ObservedGeneration: cr.Generation,
		}
	}

	phase, err := mapPhase(resp.GetPhase())
	if err != nil {
		return pluginsv1.PluginInstallationStatus{
			Phase:              pluginsv1.PluginPhaseDegraded,
			Message:            err.Error(),
			ObservedGeneration: cr.Generation,
		}
	}

	return pluginsv1.PluginInstallationStatus{
		Phase:              phase,
		Message:            resp.GetMessage(),
		Ready:              phase == pluginsv1.PluginPhaseRunning,
		ObservedGeneration: cr.Generation,
	}
}

// hasSettled reports whether the plugin has been up (Running or Degraded) since
// the current spec was applied, as the Progressing condition records it.
func hasSettled(cr *pluginsv1.PluginInstallation) bool {
	progressing := meta.FindStatusCondition(cr.Status.Conditions, ConditionProgressing)
	return progressing != nil &&
		progressing.Status == metav1.ConditionFalse &&
		progressing.ObservedGeneration == cr.Generation
}

func mapPhase(phase string) (pluginsv1.PluginPhase, error) {
	switch phase {
	case "running":
		return pluginsv1.PluginPhaseRunning, nil
	case "installing":
		return pluginsv1.PluginPhaseDeploying, nil
	case "degraded":
		return pluginsv1.PluginPhaseDegraded, nil
	case "failed":
		return pluginsv1.PluginPhaseFailed, nil
	case "uninstalling":
		return pluginsv1.PluginPhaseTerminating, nil
	default:
		// Not a panic: phase comes from external plugin input over the network,
		// so unknown values are expected when plugins use a newer SDK version.
		return "", fmt.Errorf("unknown plugin phase: %q", phase)
	}
}
