package controller

import (
	"context"
	"fmt"
	"net/http"

	"connectrpc.com/connect"

	pluginmetadatav1 "github.com/fundament-oss/fundament/plugin-sdk/metadata/proto/gen/v1"
	"github.com/fundament-oss/fundament/plugin-sdk/metadata/proto/gen/v1/pluginmetadatav1connect"

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
	return &statusPoller{
		httpClient: http.DefaultClient,
	}
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

	phase := mapPhase(resp.Msg.GetPhase())

	return pluginsv1.PluginInstallationStatus{
		Phase:              phase,
		Message:            resp.Msg.GetMessage(),
		Ready:              phase == pluginsv1.PluginPhaseRunning,
		ObservedGeneration: cr.Generation,
		PluginVersion:      resp.Msg.GetVersion(),
	}
}

func mapPhase(phase string) pluginsv1.PluginPhase {
	switch phase {
	case "running":
		return pluginsv1.PluginPhaseRunning
	case "installing":
		return pluginsv1.PluginPhaseDeploying
	case "degraded":
		return pluginsv1.PluginPhaseDegraded
	case "failed":
		return pluginsv1.PluginPhaseFailed
	default:
		panic(fmt.Sprintf("unknown plugin phase: %q", phase))
	}
}
