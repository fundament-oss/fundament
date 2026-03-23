package controller

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"connectrpc.com/connect"

	pluginmetadatav1 "github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/metadata/proto/gen/v1"
	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/metadata/proto/gen/v1/pluginmetadatav1connect"

	pluginsv1 "github.com/fundament-oss/fundament/plugin-controller/pkg/api/v1"
)

func pluginServiceURL(pluginName string) string {
	ns := pluginNamespace(pluginName)
	return fmt.Sprintf("http://plugin-%s.%s.svc.cluster.local:8080", pluginName, ns)
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
	url := pluginServiceURL(cr.Spec.PluginName)
	client := pluginmetadatav1connect.NewPluginMetadataServiceClient(s.httpClient, url)

	resp, err := client.GetStatus(ctx, connect.NewRequest(&pluginmetadatav1.GetStatusRequest{}))
	if err != nil {
		return pluginsv1.PluginInstallationStatus{
			Phase:              pluginsv1.PluginPhaseDeploying,
			Message:            fmt.Sprintf("plugin not reachable: %v", err),
			ObservedGeneration: cr.Generation,
		}
	}

	phase, err := mapPhase(resp.Msg.GetPhase())
	if err != nil {
		return pluginsv1.PluginInstallationStatus{
			Phase:              pluginsv1.PluginPhaseDegraded,
			Message:            err.Error(),
			ObservedGeneration: cr.Generation,
			PluginVersion:      resp.Msg.GetVersion(),
		}
	}

	return pluginsv1.PluginInstallationStatus{
		Phase:              phase,
		Message:            resp.Msg.GetMessage(),
		Ready:              phase == pluginsv1.PluginPhaseRunning,
		ObservedGeneration: cr.Generation,
		PluginVersion:      resp.Msg.GetVersion(),
	}
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
