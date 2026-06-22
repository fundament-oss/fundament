package controller

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openfscv1 "github.com/fundament-oss/fundament/openfsc-operator/api/v1"
)

// peerOrganization is the certificate subject organization of everything the
// operator mints for an installation. FSC surfaces it as the peer's
// human-readable name, so the team namespace is the most useful identity.
func peerOrganization(inst *openfscv1.FSCInstallation) string {
	return inst.Namespace
}

// newCertificate builds a cert-manager.io/v1 Certificate as an unstructured
// object. org/serial are set only for the federation (group) certs.
func newCertificate(ns, name, secret, issuer, commonName string, dnsNames []string, org, serial string) *unstructured.Unstructured {
	spec := map[string]any{
		"secretName": secret,
		"commonName": commonName,
		"dnsNames":   stringsToAny(dnsNames),
		"issuerRef":  map[string]any{"name": issuer, "kind": "Issuer"},
	}
	if org != "" {
		spec["subject"] = map[string]any{
			"organizations": []any{org},
			"serialNumber":  serial,
		}
	}
	return newUnstructured("cert-manager.io/v1", "Certificate", ns, name, spec)
}

// gatewayCertResources renders a gateway's Certificates: always the internal
// mTLS cert (issued by the umbrella's internal Issuer), and in Self mode also
// the group cert (issued by the operator's group Issuer; External gateways
// present a referenced Secret instead). extraHosts adds SANs for address
// overrides — the inway refuses to start unless its selfAddress host is in its
// group cert.
func gatewayCertResources(inst *openfscv1.FSCInstallation, release string, extraHosts []string) []*unstructured.Unstructured {
	ns := inst.Namespace
	host := fmt.Sprintf("%s.%s", release, ns)

	certs := []*unstructured.Unstructured{
		newCertificate(ns, certName(release, "internal"), certSecret(release, "internal"), internalIssuer,
			release+"-internal", []string{host}, "", ""),
	}
	if inst.Spec.Directory.Mode == openfscv1.DirectoryModeSelf {
		dnsNames := append([]string{host, host + ".svc.cluster.local"}, extraHosts...)
		certs = append(certs, newCertificate(ns, certName(release, "group"), certSecret(release, "group"), groupIssuer,
			host, dnsNames, peerOrganization(inst), inst.Spec.PeerID))
	}
	return certs
}

// ensureGatewayCerts reports whether all of the gateway's Certificates have a
// Ready=True condition, (re)applying them when apply is set or one is missing
// — steady-state reconciles only read.
func ensureGatewayCerts(ctx context.Context, c client.Client, inst *openfscv1.FSCInstallation, release string, extraHosts []string, apply bool) (bool, error) {
	ready := true
	for _, want := range gatewayCertResources(inst, release, extraHosts) {
		got := &unstructured.Unstructured{}
		got.SetAPIVersion("cert-manager.io/v1")
		got.SetKind("Certificate")
		err := c.Get(ctx, types.NamespacedName{Namespace: inst.Namespace, Name: want.GetName()}, got)
		missing := apierrors.IsNotFound(err)
		if err != nil && !missing {
			return false, fmt.Errorf("get certificate %q: %w", want.GetName(), err)
		}
		if apply || missing {
			if err := c.Apply(ctx, client.ApplyConfigurationFromUnstructured(want), client.ForceOwnership, client.FieldOwner(fieldOwner)); err != nil {
				return false, fmt.Errorf("apply certificate %q: %w", want.GetName(), err)
			}
			if err := c.Get(ctx, types.NamespacedName{Namespace: inst.Namespace, Name: want.GetName()}, got); err != nil {
				return false, fmt.Errorf("get certificate %q: %w", want.GetName(), err)
			}
		}
		if !certReady(got) {
			ready = false
		}
	}
	return ready, nil
}

// deleteGatewayCerts removes a gateway's Certificates (their secrets are
// garbage-collected by cert-manager). It deletes both kinds regardless of
// directory mode, so the orphan sweep needs no mode knowledge.
func deleteGatewayCerts(ctx context.Context, c client.Client, ns, release string) error {
	for _, kind := range []string{"group", "internal"} {
		u := &unstructured.Unstructured{}
		u.SetAPIVersion("cert-manager.io/v1")
		u.SetKind("Certificate")
		u.SetNamespace(ns)
		u.SetName(certName(release, kind))
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
