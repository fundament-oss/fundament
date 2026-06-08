package main

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/yaml"

	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime"
	pluginerrors "github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/errors"
	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/helpers/controllerruntime"
	openfscv1 "github.com/fundament-oss/fundament/plugins/openfsc/api/v1"
	"github.com/fundament-oss/fundament/plugins/openfsc/controllerclient"
)

//go:embed crds/*.yaml
var crdFS embed.FS

// selfPeerName is the name of the auto-seeded Peer representing this cluster's
// directory.
const selfPeerName = "self"

// Names of the default Inway/Outway the plugin ships with (auto-seeded on
// startup, then provisioned by the gateway reconcilers). They must be distinct:
// the helm release name is the CR's metadata.name, so a shared name would
// collide.
const (
	defaultInwayName  = "default-inway"
	defaultOutwayName = "default-outway"
)

// crdNames are the openfsc.fundament.io CRDs the plugin owns and must be
// Established before the operator's informers start.
var crdNames = []string{
	"peers.openfsc.fundament.io",
	"inways.openfsc.fundament.io",
	"outways.openfsc.fundament.io",
}

// buildScheme registers everything the operator needs: core/apps types, the
// apiextensions types (to apply our CRDs) and the openfsc.fundament.io/v1 types.
func buildScheme() (*runtime.Scheme, error) {
	scheme := runtime.NewScheme()
	for _, add := range []func(*runtime.Scheme) error{
		clientgoscheme.AddToScheme,
		apiextensionsv1.AddToScheme,
		openfscv1.AddToScheme,
	} {
		if err := add(scheme); err != nil {
			return nil, err
		}
	}
	return scheme, nil
}

// applyCRDs server-side-applies the embedded openfsc.fundament.io CRD manifests.
func applyCRDs(ctx context.Context, c client.Client) error {
	entries, err := fs.Glob(crdFS, "crds/*.yaml")
	if err != nil {
		return err
	}
	for _, path := range entries {
		data, err := crdFS.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		var crd apiextensionsv1.CustomResourceDefinition
		if err := yaml.Unmarshal(data, &crd); err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		if err := c.Patch(ctx, &crd, client.Apply, client.ForceOwnership, client.FieldOwner("openfsc-plugin")); err != nil {
			return fmt.Errorf("apply CRD %s: %w", crd.Name, err)
		}
	}
	return nil
}

// waitEstablished blocks until every named CRD reports the Established condition.
func waitEstablished(ctx context.Context, c client.Client, names []string) error {
	return wait.PollUntilContextTimeout(ctx, 2*time.Second, 60*time.Second, true, func(ctx context.Context) (bool, error) {
		for _, name := range names {
			var crd apiextensionsv1.CustomResourceDefinition
			if err := c.Get(ctx, types.NamespacedName{Name: name}, &crd); err != nil {
				return false, nil //nolint:nilerr // not yet visible; keep polling
			}
			established := false
			for _, cond := range crd.Status.Conditions {
				if cond.Type == apiextensionsv1.Established && cond.Status == apiextensionsv1.ConditionTrue {
					established = true
				}
			}
			if !established {
				return false, nil
			}
		}
		return true, nil
	})
}

// seedSelfPeer creates (or updates the spec of) the "self" Peer representing
// this cluster's directory, so the install is immediately testable.
func (p *OpenFSCPlugin) seedSelfPeer(ctx context.Context, c client.Client) error {
	spec := openfscv1.PeerSpec{
		GroupID:        p.cfg.GroupID,
		PeerID:         p.cfg.DirectoryPeerID,
		ManagerAddress: p.cfg.ManagerAddress,
		Directory:      true,
	}

	var peer openfscv1.Peer
	err := c.Get(ctx, types.NamespacedName{Name: selfPeerName}, &peer)
	if apierrors.IsNotFound(err) {
		peer = openfscv1.Peer{
			ObjectMeta: metav1.ObjectMeta{Name: selfPeerName},
			Spec:       spec,
		}
		if err := c.Create(ctx, &peer); err != nil {
			return fmt.Errorf("create self peer: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("get self peer: %w", err)
	}

	peer.Spec = spec
	if err := c.Update(ctx, &peer); err != nil {
		return fmt.Errorf("update self peer: %w", err)
	}
	return nil
}

// seedDefaultGateways creates the default Inway and Outway the plugin ships with
// if they are absent, so a fresh install comes with a registered gateway pair.
// Existing CRs are left untouched (the user may have edited them); a deleted
// default is re-seeded on the next operator start.
func (p *OpenFSCPlugin) seedDefaultGateways(ctx context.Context, c client.Client) error {
	inway := &openfscv1.Inway{
		ObjectMeta: metav1.ObjectMeta{Name: defaultInwayName},
		Spec:       openfscv1.InwaySpec{InwayName: defaultInwayName},
	}
	if err := c.Get(ctx, types.NamespacedName{Name: defaultInwayName}, &openfscv1.Inway{}); apierrors.IsNotFound(err) {
		if err := c.Create(ctx, inway); err != nil {
			return fmt.Errorf("create default inway: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("get default inway: %w", err)
	}

	outway := &openfscv1.Outway{
		ObjectMeta: metav1.ObjectMeta{Name: defaultOutwayName},
		Spec:       openfscv1.OutwaySpec{OutwayName: defaultOutwayName},
	}
	if err := c.Get(ctx, types.NamespacedName{Name: defaultOutwayName}, &openfscv1.Outway{}); apierrors.IsNotFound(err) {
		if err := c.Create(ctx, outway); err != nil {
			return fmt.Errorf("create default outway: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("get default outway: %w", err)
	}
	return nil
}

// runOperator applies the CRDs, seeds the self peer and runs the
// controller-runtime manager that reconciles Peer resources. It blocks until
// ctx is cancelled.
func (p *OpenFSCPlugin) runOperator(ctx context.Context, host pluginruntime.Host) error {
	host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseInstalling, Message: "applying openfsc.fundament.io CRDs"})

	if err := applyCRDs(ctx, p.kube); err != nil {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseDegraded, Message: err.Error()})
		return fmt.Errorf("apply CRDs: %w", pluginerrors.NewTransient(err))
	}
	if err := waitEstablished(ctx, p.kube, crdNames); err != nil {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseDegraded, Message: err.Error()})
		return fmt.Errorf("wait for CRDs to be established: %w", pluginerrors.NewTransient(err))
	}
	if err := p.seedSelfPeer(ctx, p.kube); err != nil {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseDegraded, Message: err.Error()})
		return fmt.Errorf("seed self peer: %w", pluginerrors.NewTransient(err))
	}
	if err := p.seedDefaultGateways(ctx, p.kube); err != nil {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseDegraded, Message: err.Error()})
		return fmt.Errorf("seed default gateways: %w", pluginerrors.NewTransient(err))
	}

	// The plugin runtime already serves :8080; disable the manager's metrics and
	// health servers and leader election (single replica per cluster).
	mgr, err := controllerruntime.SetupManager(p.scheme, &ctrl.Options{
		Metrics:                metricsserver.Options{BindAddress: "0"},
		HealthProbeBindAddress: "0",
		LeaderElection:         false,
	})
	if err != nil {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseFailed, Message: err.Error()})
		return fmt.Errorf("set up manager: %w", pluginerrors.NewPermanent(err))
	}

	reconciler := &PeerReconciler{client: mgr.GetClient(), namespace: p.cfg.Namespace, controllerURL: p.cfg.ControllerURL}
	if err := reconciler.setupWithManager(mgr); err != nil {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseFailed, Message: err.Error()})
		return fmt.Errorf("register Peer controller: %w", pluginerrors.NewPermanent(err))
	}

	// The Inway/Outway reconcilers provision gateways (certs + vendored chart per
	// CR) and observe their registration via the Controller Administration API.
	// The admin client is built lazily so a mTLS Secret that cert-manager issues
	// shortly after startup is picked up on a later requeue. Cert-manager
	// Certificates are read/written through the direct (uncached) client p.kube.
	admin := p.buildAdminClient(ctx, host)
	inway := &InwayReconciler{
		client: mgr.GetClient(), certs: p.kube, api: admin,
		chartPath: inwayChartPath, namespace: p.cfg.Namespace,
		groupID: p.cfg.GroupID, peerID: p.cfg.DirectoryPeerID,
	}
	if err := inway.setupWithManager(mgr); err != nil {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseFailed, Message: err.Error()})
		return fmt.Errorf("register Inway controller: %w", pluginerrors.NewPermanent(err))
	}
	outway := &OutwayReconciler{
		client: mgr.GetClient(), certs: p.kube, api: admin,
		chartPath: outwayChartPath, namespace: p.cfg.Namespace,
		groupID: p.cfg.GroupID, peerID: p.cfg.DirectoryPeerID,
	}
	if err := outway.setupWithManager(mgr); err != nil {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseFailed, Message: err.Error()})
		return fmt.Errorf("register Outway controller: %w", pluginerrors.NewPermanent(err))
	}

	host.ReportReady()
	host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseRunning, Message: "OpenFSC directory running"})

	if err := mgr.Start(ctx); err != nil {
		return fmt.Errorf("controller manager exited: %w", pluginerrors.NewTransient(err))
	}
	return nil
}

// defaultControllerInternalSecret is the controller's own internal TLS Secret as
// named by the `shared` umbrella (certificates.internal.existingSecret). The
// Administration API accepts this identity, so the operator reuses it as its
// mTLS client bundle unless FUNP_FSC_CERT_SECRET overrides it.
const defaultControllerInternalSecret = "shared-directory-controller-internal-tls"

// errAdminNotConfigured signals that the Controller Administration API client
// could not be built yet because its mTLS Secret is unavailable. The Inway/Outway
// reconcilers map it to a NotConfigured status and retry on the next requeue, so
// a Secret that cert-manager issues shortly after startup is picked up without a
// pod restart.
var errAdminNotConfigured = errors.New("OpenFSC Administration API not configured")

// lazyAdminClient builds the Controller Administration API client on first
// successful use, retrying while its mTLS Secret is still being issued. It
// satisfies inwayAdminAPI and outwayAdminAPI.
type lazyAdminClient struct {
	build func() (*controllerclient.Client, error)
	mu    sync.Mutex
	cl    *controllerclient.Client
}

func (l *lazyAdminClient) client() (*controllerclient.Client, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.cl != nil {
		return l.cl, nil
	}
	cl, err := l.build()
	if err != nil {
		return nil, err
	}
	l.cl = cl
	return cl, nil
}

func (l *lazyAdminClient) ListInways(ctx context.Context) ([]controllerclient.Inway, error) {
	cl, err := l.client()
	if err != nil {
		return nil, err
	}
	return cl.ListInways(ctx)
}

func (l *lazyAdminClient) ListOutways(ctx context.Context) ([]controllerclient.Outway, error) {
	cl, err := l.client()
	if err != nil {
		return nil, err
	}
	return cl.ListOutways(ctx)
}

// buildAdminClient returns a lazily-constructed Controller Administration API
// client. Address and client Secret default to the directory peer's controller
// in the plugin namespace; the mTLS bundle is read on first use so that a Secret
// issued slightly after startup is still picked up.
func (p *OpenFSCPlugin) buildAdminClient(ctx context.Context, host pluginruntime.Host) *lazyAdminClient {
	secretRef := p.cfg.FSCCertSecret
	if secretRef == "" {
		secretRef = fmt.Sprintf("%s/%s", p.cfg.Namespace, defaultControllerInternalSecret)
	}
	addr := p.cfg.ControllerAdminAddress
	if addr == "" {
		addr = fmt.Sprintf("https://shared-open-fsc-controller.%s:9444", p.cfg.Namespace)
	}
	log := host.Logger()
	build := func() (*controllerclient.Client, error) {
		cert, key, ca, err := readCertSecret(ctx, p.kube, secretRef)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", errAdminNotConfigured, err)
		}
		opts := []controllerclient.Option{controllerclient.WithClientCertificatePEM(cert, key)}
		if ca != "" {
			opts = append(opts, controllerclient.WithCACertificatePEM(ca))
		}
		if p.cfg.ControllerServerName != "" {
			opts = append(opts, controllerclient.WithServerName(p.cfg.ControllerServerName))
		}
		if p.cfg.FSCInsecure {
			opts = append(opts, controllerclient.WithInsecureSkipVerify())
		}
		cl, err := controllerclient.New(addr, opts...)
		if err != nil {
			return nil, err
		}
		log.Info("OpenFSC Administration API client configured", "address", addr, "serverName", p.cfg.ControllerServerName)
		return cl, nil
	}
	log.Info("OpenFSC Administration API client will connect lazily", "address", addr, "certSecret", secretRef)
	return &lazyAdminClient{build: build}
}

// readCertSecret loads an mTLS bundle from a "namespace/name" Secret reference,
// returning the tls.crt / tls.key / ca.crt PEM blocks. The private key is read
// from the cluster at startup rather than carried in pod env (where it would be
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
