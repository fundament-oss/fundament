package kube

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/fundament-oss/fundament/kube-api-proxy/pkg/gardener"
)

func decodeStatus(t *testing.T, rec *httptest.ResponseRecorder) metav1.Status {
	t.Helper()
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	var status metav1.Status
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &status))
	assert.Equal(t, "Status", status.Kind)
	assert.Equal(t, metav1.StatusFailure, status.Status)
	assert.Equal(t, int32(rec.Code), status.Code) //nolint:gosec // HTTP status codes fit
	return status
}

func TestWriteStepError(t *testing.T) {
	t.Parallel()

	gardenErr := apierrors.NewInvalid(schema.GroupKind{Group: "core.gardener.cloud", Kind: "Shoot"}, "s1", nil)

	tests := []struct {
		name     string
		err      error
		contains string
		excludes string
	}{
		{"sync pending", fmt.Errorf("wrapped: %w", gardener.ErrSyncPending), StepUserCredentials.Pending, ""},
		{"no shoot yet", fmt.Errorf("get admin kubeconfig: %w", gardener.ErrShootNotFound), "Gardener has no shoot for this cluster yet", ""},
		{"api error passed on", fmt.Errorf("get admin kubeconfig: %w", gardenErr), gardenErr.Status().Message, ""},
		{"transport error hidden", errors.New(`Post "https://10.1.2.3:443/apis": dial tcp 10.1.2.3:443: connection refused`), "kube-api-proxy logs", "10.1.2.3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rec := httptest.NewRecorder()
			WriteStepError(rec, StepUserCredentials, tt.err)

			assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
			status := decodeStatus(t, rec)
			assert.Equal(t, metav1.StatusReasonServiceUnavailable, status.Reason)
			assert.Contains(t, status.Message, tt.contains)
			if tt.excludes != "" {
				assert.NotContains(t, rec.Body.String(), tt.excludes)
			}
		})
	}
}

// TestWriteStepError_PendingNamesWhoseAccess pins that a pending proxy
// identity is not reported as the user's access being set up.
func TestWriteStepError_PendingNamesWhoseAccess(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	WriteStepError(rec, StepProxyCredentials, fmt.Errorf("wrapped: %w", gardener.ErrSyncPending))

	status := decodeStatus(t, rec)
	assert.Equal(t, StepProxyCredentials.Pending, status.Message)
	assert.NotContains(t, status.Message, "your access")
}

// TestMultiClusterProxy_FailsClosedWithoutToken pins that a request without
// an SA token is refused instead of being forwarded anonymously.
func TestMultiClusterProxy_FailsClosedWithoutToken(t *testing.T) {
	t.Parallel()
	m := &MultiClusterProxy{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/pods", http.NoBody)
	req = req.WithContext(context.WithValue(req.Context(), ClusterIDContextKey{}, "c1"))
	rec := httptest.NewRecorder()
	m.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	decodeStatus(t, rec)
}
