package kube

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// genClientCert returns a self-signed client certificate/key pair in PEM form.
func genClientCert(t *testing.T) (certPEM, keyPEM []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test-client", Organization: []string{"test-admins"}},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM
}

func buildKubeconfig(server string, caPEM, certPEM, keyPEM []byte) []byte {
	b64 := base64.StdEncoding.EncodeToString
	return []byte(fmt.Sprintf(`apiVersion: v1
kind: Config
clusters:
- name: test
  cluster:
    server: %s
    certificate-authority-data: %s
contexts:
- name: test
  context:
    cluster: test
    user: test
current-context: test
users:
- name: test
  user:
    client-certificate-data: %s
    client-key-data: %s
`, server, b64(caPEM), b64(certPEM), b64(keyPEM)))
}

// TestNewAnonymousFromBytes_DropsClientCert verifies, against a TLS server that
// records the client cert it is offered, that NewAnonymousFromBytes presents no
// client certificate while NewFromBytes still presents the kubeconfig's.
func TestNewAnonymousFromBytes_DropsClientCert(t *testing.T) {
	var mu sync.Mutex
	var peerCount int
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		peerCount = len(r.TLS.PeerCertificates)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	srv.TLS = &tls.Config{ClientAuth: tls.RequestClientCert} //nolint:gosec // test server
	srv.StartTLS()
	defer srv.Close()

	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	certPEM, keyPEM := genClientCert(t)
	kubeconfig := buildKubeconfig(srv.URL, caPEM, certPEM, keyPEM)

	get := func(t *testing.T, c *Client) int {
		t.Helper()
		mu.Lock()
		peerCount = -1
		mu.Unlock()
		resp, err := (&http.Client{Transport: c.Transport()}).Get(srv.URL)
		require.NoError(t, err)
		_, _ = io.Copy(io.Discard, resp.Body)
		require.NoError(t, resp.Body.Close())
		mu.Lock()
		defer mu.Unlock()
		return peerCount
	}

	regular, err := NewFromBytes(kubeconfig)
	require.NoError(t, err)
	assert.Positive(t, get(t, regular), "NewFromBytes should present its client certificate")

	anon, err := NewAnonymousFromBytes(kubeconfig)
	require.NoError(t, err)
	assert.Zero(t, get(t, anon), "NewAnonymousFromBytes must not present a client certificate")
}
