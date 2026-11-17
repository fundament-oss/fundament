// Package connectlimittest checks that a handler enforces the request cap.
package connectlimittest

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/binary"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"

	"github.com/fundament-oss/fundament/common/connectlimit"
)

// AssertRejectsGzipBomb POSTs procedure an unauthenticated gzip body over the
// Connect protocol and asserts resource_exhausted. Auth or the handler
// running first would answer with another code.
func AssertRejectsGzipBomb(t *testing.T, handler http.Handler, procedure string) {
	t.Helper()

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, procedure, bytes.NewReader(bomb(t)))
	req.Header.Set("Content-Type", "application/proto")
	req.Header.Set("Content-Encoding", "gzip")
	req.Header.Set("Connect-Protocol-Version", "1")
	assertConnectResourceExhausted(t, handler, req)
}

// AssertGetRejectsGzipBomb sends the bomb to a no-side-effects procedure as a
// Connect GET, where the compressed message travels in the query string.
func AssertGetRejectsGzipBomb(t *testing.T, handler http.Handler, procedure string) {
	t.Helper()

	query := url.Values{
		"connect":     {"v1"},
		"encoding":    {"proto"},
		"compression": {"gzip"},
		"base64":      {"1"},
		"message":     {base64.RawURLEncoding.EncodeToString(bomb(t))},
	}
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, procedure+"?"+query.Encode(), nil)
	assertConnectResourceExhausted(t, handler, req)
}

// AssertEveryProcedureRejectsGzipBomb sends the bomb over gRPC to every
// procedure linked into the binary, reflection included. A procedure the
// handler does not mount answers 404 and is skipped; any answer other than
// resource_exhausted means it is mounted without the cap.
func AssertEveryProcedureRejectsGzipBomb(t *testing.T, handler http.Handler) {
	t.Helper()
	AssertEveryProcedureRejectsGzipBombWithHeader(t, handler, nil)
}

// AssertEveryProcedureRejectsGzipBombWithHeader adds header to each request.
// connect-go runs interceptors before a streaming RPC reads its message, so
// streams behind auth need credentials to reach the cap.
func AssertEveryProcedureRejectsGzipBombWithHeader(t *testing.T, handler http.Handler, header http.Header) {
	t.Helper()

	body := bomb(t)
	// gRPC envelope: compressed flag, 4-byte big-endian length, message.
	envelope := make([]byte, 5, 5+len(body))
	envelope[0] = 1
	binary.BigEndian.PutUint32(envelope[1:], uint32(len(body))) //nolint:gosec // bounded by the require.Less in bomb
	envelope = append(envelope, body...)

	mounted := 0
	for _, procedure := range procedures() {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, procedure, bytes.NewReader(envelope))
		maps.Copy(req.Header, header)
		// connect-go serves gRPC streams only over HTTP/2.
		req.Proto, req.ProtoMajor, req.ProtoMinor = "HTTP/2.0", 2, 0
		req.Header.Set("Content-Type", "application/grpc")
		req.Header.Set("Grpc-Encoding", "gzip")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		res := rec.Result()
		if res.StatusCode == http.StatusNotFound {
			continue
		}
		mounted++
		status := res.Trailer.Get("Grpc-Status")
		if status == "" {
			status = res.Header.Get("Grpc-Status")
		}
		assert.Equal(t, strconv.Itoa(int(connect.CodeResourceExhausted)), status,
			"%s: grpc-message: %s%s", procedure, res.Trailer.Get("Grpc-Message"), res.Header.Get("Grpc-Message"))
	}
	assert.Positive(t, mounted, "handler mounts no known procedure")
}

func assertConnectResourceExhausted(t *testing.T, handler http.Handler, req *http.Request) {
	t.Helper()

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusTooManyRequests, rec.Code, "body: %s", rec.Body.String())
	assert.Contains(t, rec.Body.String(), connect.CodeResourceExhausted.String())
}

// procedures lists "/package.Service/Method" for every service registered
// with the protobuf runtime.
func procedures() []string {
	var out []string
	protoregistry.GlobalFiles.RangeFiles(func(fd protoreflect.FileDescriptor) bool {
		services := fd.Services()
		for i := range services.Len() {
			sd := services.Get(i)
			methods := sd.Methods()
			for j := range methods.Len() {
				out = append(out, "/"+string(sd.FullName())+"/"+string(methods.Get(j).Name()))
			}
		}
		return true
	})
	return out
}

func bomb(t *testing.T) []byte {
	t.Helper()

	body := gzipBomb()
	require.Less(t, len(body), connectlimit.DefaultReadMaxBytes, "compressed body must fit under the cap")
	return body
}

// gzipBomb is 16 MiB of zeros gzipped to about 16 KiB: past every cap, the
// registry's larger one included, while the compressed body fits under it.
var gzipBomb = sync.OnceValue(func() []byte {
	var body bytes.Buffer
	zw := gzip.NewWriter(&body)
	if _, err := zw.Write(make([]byte, 16<<20)); err != nil {
		panic(err)
	}
	if err := zw.Close(); err != nil {
		panic(err)
	}
	return body.Bytes()
})
