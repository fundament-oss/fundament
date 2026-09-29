package kube

import (
	"bufio"
	"context"
	"encoding/pem"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newUpgradeServer is an HTTP/2-capable TLS server, like a kube-apiserver:
// plain requests answer with the protocol they arrived over, and an SPDY
// upgrade, which only works over HTTP/1.1, switches protocols and echoes one
// line back.
func newUpgradeServer(t *testing.T) (*httptest.Server, []byte) {
	t.Helper()
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Upgrade") == "" {
			_, _ = io.WriteString(w, r.Proto) //nolint:gosec // test server echoes the protocol version, not user input
			return
		}
		if r.ProtoMajor != 1 {
			http.Error(w, "upgrade over "+r.Proto, http.StatusBadRequest)
			return
		}
		conn, rw, err := http.NewResponseController(w).Hijack()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer func() { _ = conn.Close() }()
		_, _ = rw.WriteString("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: SPDY/3.1\r\n\r\n")
		_ = rw.Flush()
		line, _ := rw.ReadString('\n')
		_, _ = rw.WriteString("echo " + line)
		_ = rw.Flush()
	}))
	srv.EnableHTTP2 = true
	srv.StartTLS()
	t.Cleanup(srv.Close)
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	certPEM, keyPEM := genClientCert(t)
	return srv, buildKubeconfig(srv.URL, caPEM, certPEM, keyPEM)
}

func spdyUpgradeRequest(ctx context.Context, t *testing.T, target string) *http.Request {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target+"/api/v1/namespaces/ns/pods/p/exec", http.NoBody)
	require.NoError(t, err)
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "SPDY/3.1")
	return req
}

// Plain requests keep HTTP/2; an SPDY upgrade goes over HTTP/1.1 instead of
// failing with "http2: invalid Upgrade request header".
func TestClientTransport_SPDYUpgradeUsesHTTP1(t *testing.T) {
	t.Parallel()
	srv, kubeconfig := newUpgradeServer(t)
	c, err := NewAnonymousFromBytes(kubeconfig)
	require.NoError(t, err)
	httpc := &http.Client{Transport: c.Transport()}

	plain, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+"/version", http.NoBody)
	require.NoError(t, err)
	resp, err := httpc.Do(plain)
	require.NoError(t, err)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	assert.Equal(t, "HTTP/2.0", string(body), "plain requests should keep HTTP/2")

	resp, err = c.Transport().RoundTrip(spdyUpgradeRequest(context.Background(), t, srv.URL))
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)
	stream, ok := resp.Body.(io.ReadWriter)
	require.True(t, ok, "upgraded body must be writable")
	_, err = io.WriteString(stream, "ping\n")
	require.NoError(t, err)
	line, err := bufio.NewReader(stream).ReadString('\n')
	require.NoError(t, err)
	assert.Equal(t, "echo ping\n", line)
}

// The same through the reverse proxy, the way kubectl reaches a shoot.
func TestReverseProxy_SPDYUpgrade(t *testing.T) {
	t.Parallel()
	upstream, kubeconfig := newUpgradeServer(t)
	c, err := NewAnonymousFromBytes(kubeconfig)
	require.NoError(t, err)
	target, err := url.Parse(upstream.URL)
	require.NoError(t, err)
	proxy := buildReverseProxy(target, c.Transport(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	front := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxy.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), SATokenContextKey{}, "user-sa-token")))
	}))
	t.Cleanup(front.Close)

	dialer := &net.Dialer{Timeout: 5 * time.Second}
	conn, err := dialer.DialContext(context.Background(), "tcp", strings.TrimPrefix(front.URL, "http://"))
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()
	require.NoError(t, conn.SetDeadline(time.Now().Add(5*time.Second)))
	_, err = io.WriteString(conn, "POST /api/v1/namespaces/ns/pods/p/exec HTTP/1.1\r\nHost: proxy\r\nConnection: Upgrade\r\nUpgrade: SPDY/3.1\r\n\r\n")
	require.NoError(t, err)

	br := bufio.NewReader(conn)
	status, err := br.ReadString('\n')
	require.NoError(t, err)
	require.Equal(t, "HTTP/1.1 101 Switching Protocols\r\n", status)
	for {
		h, err := br.ReadString('\n')
		require.NoError(t, err)
		if h == "\r\n" {
			break
		}
	}
	_, err = io.WriteString(conn, "ping\n")
	require.NoError(t, err)
	line, err := br.ReadString('\n')
	require.NoError(t, err)
	assert.Equal(t, "echo ping\n", line)
}
