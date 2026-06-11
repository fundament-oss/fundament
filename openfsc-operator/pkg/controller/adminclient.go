package controller

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/fundament-oss/fundament/openfsc-operator/pkg/controllerclient"
)

// The Controller Administration API of the directory installed by the operator
// (umbrella release "shared", fullnameOverride=shared). The controller's own
// internal TLS Secret is reused as the operator's mTLS client bundle — the
// Administration API accepts that identity — and the umbrella issues the
// controller's internal cert with the short service name as its only SAN, so
// dialing the cross-namespace FQDN needs the ServerName override to verify.
const (
	controllerAdminPort          = 9444
	controllerServiceName        = "shared-open-fsc-controller"
	controllerInternalSecret     = "shared-open-fsc-controller-internal-tls" //nolint:gosec // Secret resource name, not a credential
	controllerInternalCommonName = controllerServiceName
)

// errAdminNotConfigured signals that the Controller Administration API client
// could not be built yet because its mTLS Secret is unavailable. The
// Inway/Outway reconcilers map it to a NotConfigured status and retry on the
// next requeue, so a Secret that cert-manager issues shortly after the
// directory install is picked up without an operator restart.
var errAdminNotConfigured = errors.New("OpenFSC Administration API not configured")

// AdminClients builds and caches one Controller Administration API client per
// directory namespace. Clients are built lazily so an mTLS Secret that
// cert-manager issues shortly after the directory install is picked up on a
// later requeue.
type AdminClients struct {
	// reader reads the mTLS Secret with a direct (uncached) client so the
	// operator needs no informer on Secrets.
	reader client.Client

	mu      sync.Mutex
	clients map[string]*controllerclient.Client
}

// NewAdminClients returns an empty per-namespace client cache reading mTLS
// Secrets through the given (direct, uncached) client.
func NewAdminClients(reader client.Client) *AdminClients {
	return &AdminClients{reader: reader, clients: map[string]*controllerclient.Client{}}
}

// forNamespace returns the Administration API client for the directory in ns,
// building it on first use. It returns errAdminNotConfigured (wrapped) while
// the controller's internal TLS Secret is still being issued.
func (a *AdminClients) forNamespace(ctx context.Context, ns string) (*controllerclient.Client, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if cl, ok := a.clients[ns]; ok {
		return cl, nil
	}

	ref := fmt.Sprintf("%s/%s", ns, controllerInternalSecret)
	cert, key, ca, err := readCertSecret(ctx, a.reader, ref)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errAdminNotConfigured, err)
	}
	opts := []controllerclient.Option{
		controllerclient.WithClientCertificatePEM(cert, key),
		controllerclient.WithServerName(controllerInternalCommonName),
	}
	if ca != "" {
		opts = append(opts, controllerclient.WithCACertificatePEM(ca))
	}
	addr := fmt.Sprintf("https://%s.%s:%d", controllerServiceName, ns, controllerAdminPort)
	cl, err := controllerclient.New(addr, opts...)
	if err != nil {
		return nil, fmt.Errorf("create Administration API client: %w", err)
	}
	a.clients[ns] = cl
	return cl, nil
}

// readCertSecret loads an mTLS bundle from a "namespace/name" Secret reference,
// returning the tls.crt / tls.key / ca.crt PEM blocks. The private key is read
// from the cluster on use rather than carried in pod env (where it would be
// visible in describe/etcd/process listings).
func readCertSecret(ctx context.Context, c client.Client, ref string) (certPEM, keyPEM, caPEM string, err error) {
	ns, name, ok := strings.Cut(ref, "/")
	if !ok || ns == "" || name == "" {
		return "", "", "", fmt.Errorf("cert secret ref %q must be in namespace/name form", ref)
	}
	var sec corev1.Secret
	if err := c.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, &sec); err != nil {
		return "", "", "", fmt.Errorf("get secret %q: %w", ref, err)
	}
	certPEM, keyPEM, caPEM = string(sec.Data["tls.crt"]), string(sec.Data["tls.key"]), string(sec.Data["ca.crt"])
	if certPEM == "" || keyPEM == "" {
		return "", "", "", fmt.Errorf("secret %q is missing tls.crt/tls.key", ref)
	}
	return certPEM, keyPEM, caPEM, nil
}
