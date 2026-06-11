package controller

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// cert-manager issuers and CA secrets in the directory namespace. The group
// Issuer + its CA secret are created by the DirectoryReconciler; the internal
// Issuer + its CA secret are created by the open-fsc umbrella (release
// "shared", fullnameOverride=shared).
const (
	groupIssuer      = "shared"
	internalIssuer   = "shared-open-fsc-internal"
	groupCASecret    = "shared-ca-issuer"
	internalCASecret = "shared-open-fsc-internal-ca" //nolint:gosec // Secret resource name, not a credential

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
