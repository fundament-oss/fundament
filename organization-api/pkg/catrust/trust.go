// Package catrust builds transports for Gardener's per-shoot observability
// ingresses. Each shoot's ingress certificate is signed by that shoot's own
// cluster CA (gardener v1.139.4, botanist/monitoring.go: SigningCA =
// SecretNameCACluster), so trust is resolved per shoot and is additive:
// platform roots + operator bundle + the shoot's CA.
package catrust

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
)

// Trust holds the deployment-wide anchors. Nil or zero trusts only the
// platform roots.
type Trust struct {
	base *x509.CertPool // platform roots + operator bundle (PROMETHEUS_CA_FILE)
}

// New loads the optional operator bundle; an empty path is fine.
func New(caFile string) (*Trust, error) {
	base := systemPool()
	if caFile == "" {
		return &Trust{base: base}, nil
	}
	pem, err := os.ReadFile(caFile) //nolint:gosec // deployment config, not request input
	if err != nil {
		return nil, fmt.Errorf("read ca bundle: %w", err)
	}
	if !base.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("no certificates parsed from %s", caFile)
	}
	return &Trust{base: base}, nil
}

// TransportFor returns a fresh transport trusting the base anchors plus
// shootCAPEM. Not cached: callers cache the client built on it.
func (t *Trust) TransportFor(shootCAPEM []byte) *http.Transport {
	var pool *x509.CertPool
	if t == nil || t.base == nil {
		pool = systemPool()
	} else {
		pool = t.base.Clone()
	}
	pool.AppendCertsFromPEM(shootCAPEM) // unparseable → no anchor → visible TLS failure

	// A SystemCertPool-derived pool keeps platform verification on
	// darwin/windows (crypto/x509 verify.go), so RootCAs can always be set.
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
	return transport
}

func systemPool() *x509.CertPool {
	pool, err := x509.SystemCertPool()
	if err != nil {
		return x509.NewCertPool()
	}
	return pool
}
