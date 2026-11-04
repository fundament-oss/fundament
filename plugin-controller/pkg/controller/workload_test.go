package controller

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	pluginsv1 "github.com/fundament-oss/fundament/plugin-controller/pkg/api/v1"
	"github.com/fundament-oss/fundament/plugin-controller/pkg/config"
	"github.com/fundament-oss/fundament/plugin-controller/pkg/defclient"
	pb "github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/metadata/proto/gen/v1"
	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/metadata/proto/gen/v1/pluginmetadatav1connect"
)

const workloadTestCR = "acme--cert-manager"

// workloadTest describes one reconcile of a plugin with the given cluster state.
type workloadTest struct {
	objects      []client.Object
	httpClient   connect.HTTPClient
	interceptors interceptor.Funcs
	defClient    defclient.Client
	status       func(*pluginsv1.PluginInstallation)
}

func (w *workloadTest) reconcile(t *testing.T) (ctrl.Result, pluginsv1.PluginInstallation) {
	t.Helper()
	manifest, pin := sampleManifest(t)

	cr := testCR()
	cr.Name = workloadTestCR
	cr.Spec.DefinitionRef.OrganizationName = "acme"
	cr.Spec.DefinitionRef.DefinitionHash = pin
	cr.SetUID("test-uid")
	cr.Finalizers = []string{finalizerName}
	if w.status != nil {
		w.status(cr)
	}

	defClient := w.defClient
	if defClient == nil {
		defClient = fakeDefClient{manifest: manifest, hash: pin}
	}
	httpClient := w.httpClient
	if httpClient == nil {
		httpClient = unreachableHTTPClient()
	}
	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme()).
		WithObjects(append(w.objects, cr)...).
		WithStatusSubresource(cr).
		WithInterceptorFuncs(w.interceptors).
		Build()

	r := &Reconciler{
		client:              fakeClient,
		logger:              slog.Default(),
		cfg:                 config.Config{StatusPollInterval: 30 * time.Second, ProgressPollInterval: 5 * time.Second},
		statusPoller:        newStatusPoller(fakeClient).WithClient(httpClient),
		uninstallHTTPClient: http.DefaultClient,
		defClient:           defClient,
		defCache:            newDefinitionCache(),
	}

	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: cr.Name}}
	result, err := r.Reconcile(context.Background(), req)
	require.NoError(t, err)

	var got pluginsv1.PluginInstallation
	require.NoError(t, fakeClient.Get(context.Background(), req.NamespacedName, &got))
	return result, got
}

func testDeployment(mutate ...func(*appsv1.Deployment)) *appsv1.Deployment {
	deploy := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{
		Name:      childResourceName,
		Namespace: pluginNamespace(workloadTestCR),
	}}
	for _, m := range mutate {
		m(deploy)
	}
	return deploy
}

func withDeploymentCondition(condType appsv1.DeploymentConditionType, status corev1.ConditionStatus, reason, message string) func(*appsv1.Deployment) {
	return func(deploy *appsv1.Deployment) {
		deploy.Status.Conditions = append(deploy.Status.Conditions, appsv1.DeploymentCondition{
			Type: condType, Status: status, Reason: reason, Message: message,
		})
	}
}

func testPod(name string, mutate ...func(*corev1.Pod)) *corev1.Pod {
	cr := &pluginsv1.PluginInstallation{ObjectMeta: metav1.ObjectMeta{Name: workloadTestCR}}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name:              name,
		Namespace:         pluginNamespace(workloadTestCR),
		Labels:            selectorLabels(cr),
		CreationTimestamp: metav1.NewTime(time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)),
	}}
	for _, m := range mutate {
		m(pod)
	}
	return pod
}

func withWaitingContainer(reason, message string) func(*corev1.Pod) {
	return func(pod *corev1.Pod) {
		pod.Status.ContainerStatuses = []corev1.ContainerStatus{{
			Name:  childResourceName,
			State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: reason, Message: message}},
		}}
	}
}

func withUnschedulable(message string) func(*corev1.Pod) {
	return func(pod *corev1.Pod) {
		pod.Status.Conditions = []corev1.PodCondition{{
			Type: corev1.PodScheduled, Status: corev1.ConditionFalse, Reason: corev1.PodReasonUnschedulable, Message: message,
		}}
	}
}

func TestReconcile_UnstartablePodIsDegraded(t *testing.T) {
	tests := map[string]struct {
		objects []client.Object
		want    []string
	}{
		"image pull backoff": {
			objects: []client.Object{testPod("p", withWaitingContainer("ImagePullBackOff", `Back-off pulling image "x:1"`))},
			want:    []string{"ImagePullBackOff", `Back-off pulling image "x:1"`},
		},
		"err image pull": {
			objects: []client.Object{testPod("p", withWaitingContainer("ErrImagePull", "manifest unknown"))},
			want:    []string{"ErrImagePull", "manifest unknown"},
		},
		"err image never pull": {
			objects: []client.Object{testPod("p", withWaitingContainer("ErrImageNeverPull", "image not present with pull policy of Never"))},
			want:    []string{"ErrImageNeverPull", "image not present with pull policy of Never"},
		},
		"invalid image name": {
			objects: []client.Object{testPod("p", withWaitingContainer("InvalidImageName", "bad ref"))},
			want:    []string{"InvalidImageName", "bad ref"},
		},
		"crash loop": {
			objects: []client.Object{testPod("p", withWaitingContainer("CrashLoopBackOff", "back-off 5m0s"))},
			want:    []string{"CrashLoopBackOff", "back-off 5m0s"},
		},
		"create container error": {
			objects: []client.Object{testPod("p", withWaitingContainer("CreateContainerError", "boom"))},
			want:    []string{"CreateContainerError", "boom"},
		},
		"create container config error": {
			objects: []client.Object{testPod("p", withWaitingContainer("CreateContainerConfigError", "secret missing"))},
			want:    []string{"CreateContainerConfigError", "secret missing"},
		},
		"unschedulable": {
			objects: []client.Object{testPod("p", withUnschedulable("0/3 nodes are available"))},
			want:    []string{corev1.PodReasonUnschedulable, "0/3 nodes are available"},
		},
		"progress deadline exceeded": {
			objects: []client.Object{testDeployment(withDeploymentCondition(appsv1.DeploymentProgressing, corev1.ConditionFalse, "ProgressDeadlineExceeded", "timed out progressing"))},
			want:    []string{"ProgressDeadlineExceeded", "timed out progressing"},
		},
		"pod creation rejected": {
			objects: []client.Object{testDeployment(withDeploymentCondition(appsv1.DeploymentReplicaFailure, corev1.ConditionTrue, "FailedCreate", "pods are forbidden"))},
			want:    []string{"FailedCreate", "pods are forbidden"},
		},
		"the pod's reason wins over the deployment's": {
			objects: []client.Object{
				testDeployment(withDeploymentCondition(appsv1.DeploymentProgressing, corev1.ConditionFalse, "ProgressDeadlineExceeded", "timed out progressing")),
				testPod("p", withWaitingContainer("ImagePullBackOff", "manifest unknown")),
			},
			want: []string{"ImagePullBackOff", "manifest unknown"},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			_, got := (&workloadTest{objects: tt.objects}).reconcile(t)
			assert.Equal(t, pluginsv1.PluginPhaseDegraded, got.Status.Phase)
			assert.False(t, got.Status.Ready)
			for _, want := range tt.want {
				assert.Contains(t, got.Status.Message, want)
			}
		})
	}
}

func TestReconcile_PodStillComingUpIsDeploying(t *testing.T) {
	terminating := func(pod *corev1.Pod) {
		pod.DeletionTimestamp = &metav1.Time{Time: time.Now()}
		pod.Finalizers = []string{"test/hold"}
	}
	older := func(pod *corev1.Pod) { pod.CreationTimestamp = metav1.NewTime(pod.CreationTimestamp.Add(-time.Hour)) }

	tests := map[string][]client.Object{
		"no workload yet":   nil,
		"container created": {testPod("p", withWaitingContainer("ContainerCreating", ""))},
		"not scheduled yet": {testPod("p")},
		"a terminating pod's failure is history": {
			testPod("old", withWaitingContainer("ImagePullBackOff", "x"), terminating),
			testPod("new", withWaitingContainer("ContainerCreating", "")),
		},
		"a previous revision's failure is history": {
			testPod("old", withWaitingContainer("CrashLoopBackOff", "x"), older),
			testPod("new", withWaitingContainer("ContainerCreating", "")),
		},
		"a deadline from before the latest spec change": {
			testDeployment(func(deploy *appsv1.Deployment) {
				deploy.Generation = 2
				deploy.Status.ObservedGeneration = 1
			}, withDeploymentCondition(appsv1.DeploymentProgressing, corev1.ConditionFalse, "ProgressDeadlineExceeded", "x")),
		},
		"a rollout still progressing": {
			testDeployment(withDeploymentCondition(appsv1.DeploymentProgressing, corev1.ConditionTrue, "ReplicaSetUpdated", "x")),
		},
	}

	for name, objects := range tests {
		t.Run(name, func(t *testing.T) {
			_, got := (&workloadTest{objects: objects}).reconcile(t)
			assert.Equal(t, pluginsv1.PluginPhaseDeploying, got.Status.Phase)
			assert.Contains(t, got.Status.Message, "plugin not reachable")
		})
	}
}

// A pod that never served has not "been up", so the spec stays on its way up and
// a recovery that passes through Deploying is picked up at the fast pace again.
func TestReconcile_UnstartablePodDoesNotSettleProgress(t *testing.T) {
	result, got := (&workloadTest{objects: []client.Object{
		testPod("p", withWaitingContainer("ImagePullBackOff", "manifest unknown")),
	}}).reconcile(t)

	assert.Equal(t, 30*time.Second, result.RequeueAfter, "a failing pod is not polled at the fast pace")
	progressing := meta.FindStatusCondition(got.Status.Conditions, ConditionProgressing)
	require.NotNil(t, progressing)
	assert.Equal(t, metav1.ConditionTrue, progressing.Status)
}

// A started plugin whose reconcile keeps failing must show why it is down as well
// as why the reconcile is being retried.
func TestReconcile_ReconcileErrorKeepsPodCause(t *testing.T) {
	_, got := (&workloadTest{
		defClient: fakeDefClient{err: connect.NewError(connect.CodeUnavailable, errors.New("catalog unreachable"))},
		status:    runningOnGeneration(1),
		objects:   []client.Object{testPod("p", withWaitingContainer("ImagePullBackOff", "manifest unknown"))},
	}).reconcile(t)

	assert.Equal(t, pluginsv1.PluginPhaseDegraded, got.Status.Phase)
	assert.Contains(t, got.Status.Message, "ImagePullBackOff: manifest unknown")
	assert.Contains(t, got.Status.Message, "catalog unreachable")
}

func TestReconcile_WorkloadInspectionFailureStaysDeploying(t *testing.T) {
	_, got := (&workloadTest{interceptors: interceptor.Funcs{
		List: func(context.Context, client.WithWatch, client.ObjectList, ...client.ListOption) error {
			return errors.New("pods is forbidden")
		},
	}}).reconcile(t)

	assert.Equal(t, pluginsv1.PluginPhaseDeploying, got.Status.Phase)
	assert.Contains(t, got.Status.Message, "plugin not reachable")
	assert.Contains(t, got.Status.Message, "pods is forbidden")
}

type statusOnlyHandler struct {
	pluginmetadatav1connect.UnimplementedPluginMetadataServiceHandler
	phase string
}

func (h statusOnlyHandler) GetStatus(context.Context, *pb.GetStatusRequest) (*pb.GetStatusResponse, error) {
	return &pb.GetStatusResponse{Phase: new(h.phase)}, nil
}

// A rollout's broken new pod must not override what the still-serving old pod reports.
func TestReconcile_ReachablePluginIgnoresPodState(t *testing.T) {
	_, handler := pluginmetadatav1connect.NewPluginMetadataServiceHandler(statusOnlyHandler{phase: "running"})
	httpClient := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec.Result(), nil
	})}

	_, got := (&workloadTest{
		httpClient: httpClient,
		objects:    []client.Object{testPod("new", withWaitingContainer("ImagePullBackOff", "manifest unknown"))},
	}).reconcile(t)

	assert.Equal(t, pluginsv1.PluginPhaseRunning, got.Status.Phase)
	assert.True(t, got.Status.Ready)
}
