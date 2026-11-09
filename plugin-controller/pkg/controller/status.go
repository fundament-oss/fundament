package controller

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"connectrpc.com/connect"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	pluginmetadatav1 "github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/metadata/proto/gen/v1"
	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/metadata/proto/gen/v1/pluginmetadatav1connect"

	pluginsv1 "github.com/fundament-oss/fundament/plugin-controller/pkg/api/v1"
)

// waitingForPluginMessage is the status message of an installation whose plugin
// has not answered yet since the current spec was applied.
const waitingForPluginMessage = "waiting for the plugin to start"

func pluginServiceURL(installationName string) string {
	// The Service is constant-named inside the plugin's own namespace.
	return fmt.Sprintf("http://%s.%s.svc.cluster.local:8080", childResourceName, pluginNamespace(installationName))
}

type statusPoller struct {
	httpClient connect.HTTPClient
	reader     client.Reader
}

func newStatusPoller(reader client.Reader) *statusPoller {
	return &statusPoller{httpClient: &http.Client{Timeout: 5 * time.Second}, reader: reader}
}

func (s *statusPoller) WithClient(client connect.HTTPClient) *statusPoller {
	s.httpClient = client

	return s
}

// poll returns the installation's status and whether the plugin answered. A plugin
// that answers is the authority on its own health; otherwise the workload says
// whether it is still coming up or will not come up without intervention.
func (s *statusPoller) poll(ctx context.Context, cr *pluginsv1.PluginInstallation) (pluginsv1.PluginInstallationStatus, bool) {
	url := pluginServiceURL(cr.Name)
	rpcClient := pluginmetadatav1connect.NewPluginMetadataServiceClient(s.httpClient, url)

	resp, err := rpcClient.GetStatus(ctx, &pluginmetadatav1.GetStatusRequest{})
	if err != nil {
		return s.unreachableStatus(ctx, cr, err), false
	}

	phase, err := mapPhase(resp.GetPhase())
	if err != nil {
		return pluginsv1.PluginInstallationStatus{
			Phase:              pluginsv1.PluginPhaseDegraded,
			Message:            err.Error(),
			ObservedGeneration: cr.Generation,
		}, true
	}

	return pluginsv1.PluginInstallationStatus{
		Phase:              phase,
		Message:            resp.GetMessage(),
		Ready:              phase == pluginsv1.PluginPhaseRunning,
		ObservedGeneration: cr.Generation,
	}, true
}

func (s *statusPoller) unreachableStatus(ctx context.Context, cr *pluginsv1.PluginInstallation, rpcErr error) pluginsv1.PluginInstallationStatus {
	// Before the current spec first came up, an unreachable plugin is one whose
	// pod is still starting. After, it is one that stopped answering, typically
	// a crash loop: reporting that as Deploying would show a broken plugin as
	// one still being installed.
	//
	// While it is starting, not answering is expected, and the dial error
	// (connection refused, no such host) says nothing about what is wrong. The
	// console shows this message during every install, so it says what is
	// happening instead. The workload inspection below still replaces it with
	// the real cause when the pod cannot start.
	phase := pluginsv1.PluginPhaseDeploying
	message := waitingForPluginMessage
	if hasSettled(cr) {
		phase = pluginsv1.PluginPhaseDegraded
		message = fmt.Sprintf("plugin not reachable: %v", rpcErr)
	}
	status := pluginsv1.PluginInstallationStatus{
		Phase:              phase,
		Message:            message,
		ObservedGeneration: cr.Generation,
	}

	failure, err := s.inspectWorkload(ctx, cr)
	switch {
	case err != nil:
		status.Message = fmt.Sprintf("%s; inspect workload: %v", status.Message, err)
	case failure != "":
		status.Phase = pluginsv1.PluginPhaseDegraded
		status.Message = failure
	}
	return status
}

// hasSettled reports whether the plugin has been up (Running or Degraded) since
// the current spec was applied, as the Progressing condition records it.
func hasSettled(cr *pluginsv1.PluginInstallation) bool {
	if progressing := meta.FindStatusCondition(cr.Status.Conditions, ConditionProgressing); progressing != nil {
		return progressing.Status == metav1.ConditionFalse &&
			progressing.ObservedGeneration == cr.Generation
	}
	// A status written before the Progressing condition existed.
	if cr.Status.ObservedGeneration != cr.Generation {
		return false
	}
	switch cr.Status.Phase {
	case pluginsv1.PluginPhaseRunning, pluginsv1.PluginPhaseDegraded:
		return true
	default:
		return false
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
