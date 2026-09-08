package catrust

import (
	"crypto/tls"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/organization-api/pkg/catrust/certtest"
)

func writePEM(t *testing.T, content []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ca.crt")
	require.NoError(t, os.WriteFile(path, content, 0o600))
	return path
}

// get issues one request through trust and reports whether TLS verified.
func get(t *testing.T, trust *Trust, shootCAPEM []byte, url string) error {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, http.NoBody)
	require.NoError(t, err)
	resp, err := (&http.Client{Transport: trust.TransportFor(shootCAPEM)}).Do(req)
	if resp != nil {
		require.NoError(t, resp.Body.Close())
	}
	if err != nil {
		return fmt.Errorf("get: %w", err)
	}
	return nil
}

func TestNew_BundleErrors(t *testing.T) {
	t.Run("missing file", func(t *testing.T) {
		_, err := New(filepath.Join(t.TempDir(), "absent.crt"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "read ca bundle")
	})

	t.Run("no certificate in file", func(t *testing.T) {
		_, err := New(writePEM(t, []byte("not a certificate")))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no certificates parsed")
	})
}

func TestTransportFor_OperatorBundleVerifiesAConnection(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	serverCA := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})

	untrusted, err := New("")
	require.NoError(t, err)
	require.Error(t, get(t, untrusted, nil, srv.URL))

	trust, err := New(writePEM(t, serverCA))
	require.NoError(t, err)
	require.NoError(t, get(t, trust, nil, srv.URL))
}

func TestTransportFor_ShootCAVerifiesAConnection(t *testing.T) {
	ca := certtest.NewCA(t)
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{ca.ServerCert(t)}, MinVersion: tls.VersionTLS12}
	srv.StartTLS()
	defer srv.Close()

	trust, err := New("")
	require.NoError(t, err)
	require.Error(t, get(t, trust, nil, srv.URL))
	require.NoError(t, get(t, trust, ca.PEM, srv.URL))
	require.Error(t, get(t, trust, certtest.NewCA(t).PEM, srv.URL))

	// A nil Trust must still honour the shoot CA.
	require.NoError(t, get(t, nil, ca.PEM, srv.URL))
}
