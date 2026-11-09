package controller

import (
	"context"
	"fmt"
	"slices"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	pluginsv1 "github.com/fundament-oss/fundament/plugin-controller/pkg/api/v1"
)

// progressDeadlineExceeded is the Progressing condition reason the Deployment
// controller sets on a stuck rollout; k8s.io/api has no constant for it.
const progressDeadlineExceeded = "ProgressDeadlineExceeded"

// stuckWaitingReasons are the container waiting reasons that need something to
// change before the container can run, unlike ContainerCreating or PodInitializing.
var stuckWaitingReasons = []string{
	"CreateContainerConfigError",
	"CreateContainerError",
	"CrashLoopBackOff",
	"ErrImageNeverPull",
	"ErrImagePull",
	"ImagePullBackOff",
	"InvalidImageName",
	"RunContainerError",
}

// inspectWorkload explains why the plugin's workload cannot come up, or returns
// "" when nothing says it won't.
func (s *statusPoller) inspectWorkload(ctx context.Context, cr *pluginsv1.PluginInstallation) (string, error) {
	namespace := pluginNamespace(cr.Name)

	var pods corev1.PodList
	if err := s.reader.List(ctx, &pods, client.InNamespace(namespace), client.MatchingLabels(selectorLabels(cr))); err != nil {
		return "", fmt.Errorf("list pods: %w", err)
	}
	if reason := podFailure(pods.Items); reason != "" {
		return reason, nil
	}

	var deploy appsv1.Deployment
	if err := s.reader.Get(ctx, client.ObjectKey{Namespace: namespace, Name: childResourceName}, &deploy); err != nil {
		if apierrors.IsNotFound(err) {
			return "", nil
		}
		return "", fmt.Errorf("get deployment: %w", err)
	}
	return deploymentFailure(&deploy), nil
}

// podFailure inspects only the newest live pod: it belongs to the current
// revision, and older pods' failures say nothing about whether it will start.
func podFailure(pods []corev1.Pod) string {
	var newest *corev1.Pod
	for i := range pods {
		pod := &pods[i]
		if pod.DeletionTimestamp.IsZero() && (newest == nil || newest.CreationTimestamp.Before(&pod.CreationTimestamp)) {
			newest = pod
		}
	}
	if newest == nil {
		return ""
	}

	for i := range newest.Status.ContainerStatuses {
		container := &newest.Status.ContainerStatuses[i]
		if waiting := container.State.Waiting; waiting != nil && slices.Contains(stuckWaitingReasons, waiting.Reason) {
			return fmt.Sprintf("container %s: %s: %s", container.Name, waiting.Reason, waiting.Message)
		}
	}
	for i := range newest.Status.Conditions {
		condition := &newest.Status.Conditions[i]
		if condition.Type == corev1.PodScheduled && condition.Status == corev1.ConditionFalse && condition.Reason == corev1.PodReasonUnschedulable {
			return fmt.Sprintf("pod %s: %s: %s", newest.Name, condition.Reason, condition.Message)
		}
	}
	return ""
}

// deploymentFailure covers what no pod can report: a pod that was never created,
// or a rollout that ran out of time. Conditions from before the latest spec
// change describe a previous rollout.
func deploymentFailure(deploy *appsv1.Deployment) string {
	if deploy.Status.ObservedGeneration < deploy.Generation {
		return ""
	}
	for i := range deploy.Status.Conditions {
		condition := &deploy.Status.Conditions[i]
		stuck := condition.Type == appsv1.DeploymentProgressing && condition.Status == corev1.ConditionFalse && condition.Reason == progressDeadlineExceeded
		rejected := condition.Type == appsv1.DeploymentReplicaFailure && condition.Status == corev1.ConditionTrue
		if stuck || rejected {
			return fmt.Sprintf("deployment %s: %s: %s", deploy.Name, condition.Reason, condition.Message)
		}
	}
	return ""
}
