// Package connectlimittest checks that a handler enforces the request cap.
package connectlimittest

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"net/http"
	"net/http/httptest"
	"strconv"
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

	body := gzipBomb(t)
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, procedure, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/proto")
	req.Header.Set("Content-Encoding", "gzip")
	req.Header.Set("Connect-Protocol-Version", "1")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusTooManyRequests, rec.Code, "body: %s", rec.Body.String())
	assert.Contains(t, rec.Body.String(), connect.CodeResourceExhausted.String())
}

// AssertReflectionRejectsGzipBomb sends both gRPC reflection versions a
// gzip-compressed gRPC message that inflates past the cap and asserts
// resource_exhausted.
func AssertReflectionRejectsGzipBomb(t *testing.T, handler http.Handler) {
	t.Helper()

	body := gzipBomb(t)
	// gRPC envelope: compressed flag, 4-byte big-endian length, message.
	envelope := make([]byte, 5, 5+len(body))
	envelope[0] = 1
	binary.BigEndian.PutUint32(envelope[1:], uint32(len(body))) //nolint:gosec // bounded by the require.Less in gzipBomb
	envelope = append(envelope, body...)

	for _, procedure := range []string{
		"/grpc.reflection.v1.ServerReflection/ServerReflectionInfo",
		"/grpc.reflection.v1alpha.ServerReflection/ServerReflectionInfo",
	} {
		t.Run(procedure, func(t *testing.T) {
			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, procedure, bytes.NewReader(envelope))
			// Reflection is a bidi stream, which connect-go serves only over HTTP/2.
			req.Proto, req.ProtoMajor, req.ProtoMinor = "HTTP/2.0", 2, 0
			req.Header.Set("Content-Type", "application/grpc")
			req.Header.Set("Grpc-Encoding", "gzip")
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			res := rec.Result()
			status := res.Trailer.Get("Grpc-Status")
			if status == "" {
				status = res.Header.Get("Grpc-Status")
			}
			assert.Equal(t, strconv.Itoa(int(connect.CodeResourceExhausted)), status,
				"grpc-message: %s%s", res.Trailer.Get("Grpc-Message"), res.Header.Get("Grpc-Message"))
		})
	}
}

// gzipBomb returns 16 MiB of zeros gzipped to about 16 KiB: past every cap,
// the registry's larger one included, while the compressed body fits under it.
func gzipBomb(t *testing.T) []byte {
	t.Helper()

	var body bytes.Buffer
	zw := gzip.NewWriter(&body)
	_, err := zw.Write(make([]byte, 16<<20))
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	require.Less(t, body.Len(), connectlimit.DefaultReadMaxBytes, "compressed body must fit under the cap")
	return body.Bytes()
}
