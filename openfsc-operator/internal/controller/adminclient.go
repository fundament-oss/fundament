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

	"github.com/fundament-oss/fundament/openfsc-operator/internal/controllerclient"
)

// The controller's own internal TLS Secret is reused as the operator's mTLS
// client bundle — the Administration API accepts that identity — and the
// umbrella issues the controller's internal cert with the short service name
// as its only SAN, so the operator dialing the cross-namespace FQDN needs the
// ServerName override to verify.
const controllerAdminPort = 9444

// errAdminNotConfigured: the client cannot be built yet because the
// controller's mTLS Secret is still being issued; gateways report Pending and
// the next requeue retries.
var errAdminNotConfigured = errors.New("OpenFSC Administration API not configured")

// AdminClients caches one Controller Administration API client per
// installation namespace, built lazily from the controller's internal TLS
// Secret (read with a direct client so the operator needs no Secret informer).
type AdminClients struct {
	reader client.Client

	mu      sync.Mutex
	clients map[string]*controllerclient.Client
}

func NewAdminClients(reader client.Client) *AdminClients {
	return &AdminClients{reader: reader, clients: map[string]*controllerclient.Client{}}
}

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
		controllerclient.WithServerName(controllerService),
	}
	if ca != "" {
		opts = append(opts, controllerclient.WithCACertificatePEM(ca))
	}
	addr := fmt.Sprintf("https://%s.%s:%d", controllerService, ns, controllerAdminPort)
	cl, err := controllerclient.New(addr, opts...)
	if err != nil {
		return nil, fmt.Errorf("create Administration API client: %w", err)
	}
	a.clients[ns] = cl
	return cl, nil
}

func (a *AdminClients) forget(ns string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.clients, ns)
}

// readCertSecret loads an mTLS bundle from a "namespace/name" Secret
// reference. The private key is read from the cluster on use rather than
// carried in pod env (where it would be visible in describe/etcd/process
// listings).
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
