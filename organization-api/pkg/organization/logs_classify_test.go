package organization

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/fundament-oss/fundament/organization-api/pkg/logs"
)

// ADR-0027 requires per-cluster failures to degrade rather than fail the RPC,
// but degrading *everything* made four situations indistinguishable: a 401 that
// survived re-resolution and a 403 from Plutono both rendered indefinitely as
// "this cluster has no log backend", visible only in a warn log.
func Test_ClassifyLogError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want logErrorKind
	}{
		{"bad query surfaces to the caller",
			&logs.StatusError{StatusCode: http.StatusBadRequest}, logErrorCaller},
		{"unprocessable surfaces to the caller",
			&logs.StatusError{StatusCode: http.StatusUnprocessableEntity}, logErrorCaller},
		{"pod required surfaces to the caller", logs.ErrPodRequired, logErrorCaller},
		{"rotated credentials are a config fault",
			&logs.StatusError{StatusCode: http.StatusUnauthorized}, logErrorConfig},
		{"revoked datasource proxy is a config fault",
			&logs.StatusError{StatusCode: http.StatusForbidden}, logErrorConfig},
		{"gateway error is environmental",
			&logs.StatusError{StatusCode: http.StatusBadGateway}, logErrorEnvironmental},
		{"unavailable is environmental",
			&logs.StatusError{StatusCode: http.StatusServiceUnavailable}, logErrorEnvironmental},
		{"unreachable ingress is environmental",
			&url.Error{Op: "Get", URL: "https://seed", Err: errors.New("connection refused")},
			logErrorEnvironmental},
		{"caller going away is neither", context.Canceled, logErrorCanceled},
		// An unparseable response is a property of our integration, not of the
		// cluster: reporting an empty success would be the same silent lie this
		// classification exists to remove.
		{"unparseable response is a config fault", errors.New("decode response: unexpected EOF"), logErrorConfig},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, classifyLogError(tt.err))
		})
	}
}
