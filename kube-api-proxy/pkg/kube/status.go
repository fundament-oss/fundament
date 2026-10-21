package kube

import (
	"encoding/json"
	"errors"
	"net/http"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/fundament-oss/fundament/kube-api-proxy/pkg/gardener"
)

// WriteStatus writes a Kubernetes Status object, so kubectl prints message
// instead of `an error on the server ("...") has prevented the request from
// succeeding`. No Retry-After: client-go would retry silently.
func WriteStatus(w http.ResponseWriter, code int32, reason metav1.StatusReason, message string) {
	status := metav1.Status{
		TypeMeta: metav1.TypeMeta{Kind: "Status", APIVersion: "v1"},
		Status:   metav1.StatusFailure,
		Message:  message,
		Reason:   reason,
		Code:     code,
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(int(code))
	_ = json.NewEncoder(w).Encode(status) //nolint:errchkjson // best effort, headers are already sent
}

// Step is one of the proxy's own steps before a request reaches the cluster.
type Step struct {
	// Name says what the proxy was doing, e.g. "getting your credentials for
	// this cluster".
	Name string
	// Pending is the message while the access this step needs is still being
	// set up on the cluster.
	Pending string
}

var (
	// StepUserCredentials gets the requesting user's ServiceAccount token.
	StepUserCredentials = Step{
		Name:    "getting your credentials for this cluster",
		Pending: "your access to this cluster is still being set up, try again shortly",
	}
	// StepProxyCredentials gets kube-api-proxy's own ServiceAccount token.
	StepProxyCredentials = Step{
		Name:    "getting the platform's credentials for this cluster",
		Pending: "the platform's access to this cluster is still being set up, try again shortly",
	}
	// StepGardenerCredentials gets an admin kubeconfig from Gardener.
	StepGardenerCredentials = Step{
		Name:    "getting credentials for this cluster from Gardener",
		Pending: "access to this cluster is still being set up, try again shortly",
	}
)

// WriteStepError reports a failure in one of the proxy's own steps before a
// request reaches the cluster, typically while the cluster is still being
// created. Kubernetes API errors from Gardener or the shoot carry a message
// meant for API clients and are passed on. Any other error (transport, TLS,
// parsing) can name internal hosts and addresses, so the caller only gets a
// fixed text; the full error belongs in the log.
func WriteStepError(w http.ResponseWriter, step Step, err error) {
	message := step.Name + " failed; the details are in the kube-api-proxy logs"

	var apiStatus apierrors.APIStatus
	switch {
	case errors.Is(err, gardener.ErrSyncPending):
		message = step.Pending
	case errors.Is(err, gardener.ErrShootNotFound):
		message = step.Name + " failed: Gardener has no shoot for this cluster yet"
	case errors.As(err, &apiStatus):
		message = step.Name + " failed: " + apiStatus.Status().Message
	}

	WriteStatus(w, http.StatusServiceUnavailable, metav1.StatusReasonServiceUnavailable, message)
}
