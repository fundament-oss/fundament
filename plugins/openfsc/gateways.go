package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Gateway provisioning constants. The inway/outway workloads run in the plugin's
// namespace (alongside the directory peer's controller/manager), so short
// in-cluster service names resolve and the `shared` umbrella's cert-manager
// issuers can sign their certs.
const (
	// Vendored chart paths (see Dockerfile COPY plugins/openfsc/charts /charts).
	inwayChartPath  = "/charts/open-fsc-inway"
	outwayChartPath = "/charts/open-fsc-outway"

	// cert-manager issuers and CA secrets. The group Issuer + its CA secret are
	// created by the openfsc-directory helper chart; the internal Issuer + its CA
	// secret are created by the open-fsc umbrella (release "shared",
	// fullnameOverride=shared).
	groupIssuer      = "shared"
	internalIssuer   = "shared-open-fsc-internal"
	groupCASecret    = "shared-ca-issuer"
	internalCASecret = "shared-open-fsc-internal-ca"

	// OpenFSC component service endpoints in the peer namespace (release "shared").
	controllerRegistrationAddr = "https://shared-open-fsc-controller:9443"
	managerInternalAddr        = "https://shared-open-fsc-manager-internal:9443"
	managerUnauthAddr          = "https://shared-open-fsc-manager-internal-unauthenticated:9444"
	transactionLogAddr         = "https://shared-open-fsc-txlog-api:9443"

	// peerOrg is the directory peer's certificate organization; the gateway group
	// certs carry it (plus the peer's serial number) so they belong to the peer.
	peerOrg = "OpenFSC Directory"
)

// --- cert-manager Certificate provisioning (unstructured, no extra dep) -------

func certName(gateway, kind string) string   { return gateway + "-" + kind } // <name>-group / <name>-internal
func certSecret(gateway, kind string) string { return gateway + "-" + kind + "-tls" }

// newCertificate builds a cert-manager.io/v1 Certificate as an unstructured
// object. org/serial are set only for the federation (group) cert.
func newCertificate(ns, name, secret, issuer, commonName string, dnsNames []string, org, serial string) *unstructured.Unstructured {
	dns := make([]any, len(dnsNames))
	for i, d := range dnsNames {
		dns[i] = d
	}
	spec := map[string]any{
		"secretName": secret,
		"commonName": commonName,
		"dnsNames":   dns,
		"issuerRef":  map[string]any{"name": issuer, "kind": "Issuer"},
	}
	if org != "" {
		spec["subject"] = map[string]any{
			"organizations": []any{org},
			"serialNumber":  serial,
		}
	}
	u := &unstructured.Unstructured{Object: map[string]any{"spec": spec}}
	u.SetAPIVersion("cert-manager.io/v1")
	u.SetKind("Certificate")
	u.SetNamespace(ns)
	u.SetName(name)
	return u
}

// ensureGatewayCerts creates the gateway's group + internal Certificates if
// absent and reports whether both have a Ready=True condition. The group cert's
// SAN/selfAddress host is <gateway>.<ns>; the inway refuses to start unless its
// selfAddress host is in its group cert.
func ensureGatewayCerts(ctx context.Context, c client.Client, ns, gateway, peerID string) (bool, error) {
	host := fmt.Sprintf("%s.%s", gateway, ns)
	desired := []*unstructured.Unstructured{
		newCertificate(ns, certName(gateway, "group"), certSecret(gateway, "group"), groupIssuer,
			host, []string{host, host + ".svc.cluster.local"}, peerOrg, peerID),
		newCertificate(ns, certName(gateway, "internal"), certSecret(gateway, "internal"), internalIssuer,
			gateway+"-internal", []string{host}, "", ""),
	}
	ready := true
	for _, want := range desired {
		got := &unstructured.Unstructured{}
		got.SetAPIVersion("cert-manager.io/v1")
		got.SetKind("Certificate")
		err := c.Get(ctx, types.NamespacedName{Namespace: ns, Name: want.GetName()}, got)
		if apierrors.IsNotFound(err) {
			if err := c.Create(ctx, want); err != nil {
				return false, fmt.Errorf("create certificate %q: %w", want.GetName(), err)
			}
			ready = false
			continue
		}
		if err != nil {
			return false, fmt.Errorf("get certificate %q: %w", want.GetName(), err)
		}
		if !certReady(got) {
			ready = false
		}
	}
	return ready, nil
}

// deleteGatewayCerts removes the gateway's Certificates (their secrets are
// garbage-collected by cert-manager).
func deleteGatewayCerts(ctx context.Context, c client.Client, ns, gateway string) error {
	for _, kind := range []string{"group", "internal"} {
		u := &unstructured.Unstructured{}
		u.SetAPIVersion("cert-manager.io/v1")
		u.SetKind("Certificate")
		u.SetNamespace(ns)
		u.SetName(certName(gateway, kind))
		if err := c.Delete(ctx, u); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("delete certificate %q: %w", u.GetName(), err)
		}
	}
	return nil
}

func certReady(u *unstructured.Unstructured) bool {
	conds, _, _ := unstructured.NestedSlice(u.Object, "status", "conditions")
	for _, c := range conds {
		m, ok := c.(map[string]any)
		if ok && m["type"] == "Ready" && m["status"] == "True" {
			return true
		}
	}
	return false
}

// --- Helm values --------------------------------------------------------------

// inwayValues renders the --set values for the open-fsc-inway chart. gateway is
// the k8s release/DNS name; fscName is the FSC name the inway registers under.
func inwayValues(ns, gateway, fscName, groupID string) map[string]string {
	v := gatewayCommonValues(ns, gateway, fscName, groupID)
	v["service.type"] = "ClusterIP"
	v["service.port"] = "443" // inway requires its registered selfAddress on :443
	v["config.selfAddress"] = fmt.Sprintf("https://%s.%s:443", gateway, ns)
	v["config.managerInternalUnauthenticatedAddress"] = managerUnauthAddr
	return v
}

// outwayValues renders the --set values for the open-fsc-outway chart. An outway
// is a forward proxy: no selfAddress and no inbound server cert.
func outwayValues(ns, gateway, fscName, groupID string) map[string]string {
	v := gatewayCommonValues(ns, gateway, fscName, groupID)
	v["https.enabled"] = "false"
	v["service.type"] = "ClusterIP"
	v["config.managerInternalAddress"] = managerInternalAddr
	return v
}

func gatewayCommonValues(ns, gateway, fscName, groupID string) map[string]string {
	return map[string]string{
		"fullnameOverride": gateway,
		"image.pullPolicy": "IfNotPresent",
		"global.groupID":   groupID,
		"global.certificates.group.caCertificatePEMExistingSecret.name":    groupCASecret,
		"global.certificates.group.caCertificatePEMExistingSecret.key":     "tls.crt",
		"global.certificates.internal.caCertificatePEMExistingSecret.name": internalCASecret,
		"global.certificates.internal.caCertificatePEMExistingSecret.key":  "tls.crt",
		"config.groupID": groupID,
		"config.name":    fscName,
		"config.controllerRegistrationApiAddress": controllerRegistrationAddr,
		"config.transactionLogApiAddress":         transactionLogAddr,
		"config.disableCrlChecks":                 "true",
		"certificates.group.existingSecret":       certSecret(gateway, "group"),
		"certificates.internal.existingSecret":    certSecret(gateway, "internal"),
	}
}

// --- Helm CLI (no --wait: the reconciler confirms readiness via the admin API) -

func helmInstalled(ctx context.Context, ns, release string) (bool, error) {
	cmd := exec.CommandContext(ctx, "helm", "status", release, "--namespace", ns) //nolint:gosec // args internal
	cmd.Env = helmEnv()
	return cmd.Run() == nil, nil
}

// helmUpgradeInstall applies the chart without --wait; readiness is tracked by
// observing registration rather than blocking the reconcile worker.
func helmUpgradeInstall(ctx context.Context, ns, release, chart string, values map[string]string) error {
	args := []string{"upgrade", "--install", release, chart, "--namespace", ns}
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		args = append(args, "--set", k+"="+values[k])
	}
	return runHelm(ctx, args...)
}

func helmUninstall(ctx context.Context, ns, release string) error {
	return runHelm(ctx, "uninstall", release, "--namespace", ns, "--ignore-not-found")
}

func runHelm(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "helm", args...) //nolint:gosec // args internal
	cmd.Env = helmEnv()
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("helm %s: %s: %w", args[0], strings.TrimSpace(string(out)), err)
	}
	return nil
}

// pluginWorkDir is a stable, writable scratch dir under the OS temp dir (the
// plugin container's HOME may be read-only). The installer caches its OpenFSC
// clone here across install retries; helm/git write their state here too.
func pluginWorkDir() string {
	dir := filepath.Join(os.TempDir(), "openfsc-plugin")
	_ = os.MkdirAll(dir, 0o755)
	return dir
}

// helmEnv points HOME and the HELM_* dirs at pluginWorkDir so helm/git can write
// config, cache and repo state in the locked-down container.
func helmEnv() []string {
	work := pluginWorkDir()
	return append(os.Environ(),
		"HOME="+work,
		"HELM_CACHE_HOME="+filepath.Join(work, "helm", "cache"),
		"HELM_CONFIG_HOME="+filepath.Join(work, "helm", "config"),
		"HELM_DATA_HOME="+filepath.Join(work, "helm", "data"),
	)
}
