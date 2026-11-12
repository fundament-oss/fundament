// Package connectlimittest checks that a handler enforces the request cap.
package connectlimittest

import (
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/common/connectlimit"
)

// AssertRejectsGzipBomb sends procedure an unauthenticated gzip body that
// inflates past the cap and asserts resource_exhausted. Auth or the handler
// running first would answer with another code.
func AssertRejectsGzipBomb(t *testing.T, handler http.Handler, procedure string) {
	t.Helper()

	// 16 MiB of zeros gzips to about 16 KiB and clears every cap, the
	// registry's larger one included.
	var body bytes.Buffer
	zw := gzip.NewWriter(&body)
	_, err := zw.Write(make([]byte, 16<<20))
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	require.Less(t, body.Len(), connectlimit.DefaultReadMaxBytes, "compressed body must fit under the cap")

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, procedure, &body)
	req.Header.Set("Content-Type", "application/proto")
	req.Header.Set("Content-Encoding", "gzip")
	req.Header.Set("Connect-Protocol-Version", "1")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusTooManyRequests, rec.Code, "body: %s", rec.Body.String())
	assert.Contains(t, rec.Body.String(), connect.CodeResourceExhausted.String())
}
