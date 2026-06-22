package controller

import (
	"fmt"
	"net/url"

	openfscv1 "github.com/fundament-oss/fundament/openfsc-operator/api/v1"
)

// Every name inside an installation's namespace is fixed. The umbrella is
// installed as release "fsc" with fullnameOverride=fsc, so the umbrella's
// cert-manager objects (fsc-open-fsc-<component>-internal-tls, issued with the
// subchart service names as SANs) line up with the subchart fullnames
// (fsc-open-fsc-<component>). With one FSCInstallation per namespace, fixed
// names cannot collide and every installation's namespace looks identical.
const (
	umbrellaRelease = "fsc"

	managerDeployment    = "fsc-open-fsc-manager"
	controllerDeployment = "fsc-open-fsc-controller"

	managerExternalService = "fsc-open-fsc-manager-external"
	controllerService      = "fsc-open-fsc-controller"

	// Created by the umbrella: the internal mTLS chain shared by all components.
	// (The gosec suppressions cover Secret resource names, not credentials.)
	internalIssuer           = "fsc-open-fsc-internal"
	internalCASecret         = "fsc-open-fsc-internal-ca"                          //nolint:gosec // Secret resource name, not a credential
	managerInternalSecret    = "fsc-open-fsc-manager-internal-tls"                 //nolint:gosec // Secret resource name, not a credential
	managerUnauthSecret      = "fsc-open-fsc-manager-internal-unauthenticated-tls" //nolint:gosec // Secret resource name, not a credential
	controllerInternalSecret = "fsc-open-fsc-controller-internal-tls"              //nolint:gosec // Secret resource name, not a credential
	auditlogInternalSecret   = "fsc-open-fsc-auditlog-internal-tls"                //nolint:gosec // Secret resource name, not a credential
	txlogInternalSecret      = "fsc-open-fsc-txlog-api-internal-tls"               //nolint:gosec // Secret resource name, not a credential

	// Created by the operator in Self mode: the self-signed group (federation)
	// CA chain and the Manager's group certificate.
	groupSelfSignedIssuer  = "fsc-group-selfsigned"
	groupIssuer            = "fsc-group"
	groupCASecret          = "fsc-group-ca"          //nolint:gosec // Secret resource name, not a credential
	managerGroupCertSecret = "fsc-manager-group-tls" //nolint:gosec // Secret resource name, not a credential

	// The CloudNativePG cluster; CNPG derives the -rw service and -app secret.
	postgresCluster = "fsc-postgresql"
	postgresService = "fsc-postgresql-rw"
	postgresSecret  = "fsc-postgresql-app" //nolint:gosec // Secret resource name, not a credential
)

// In-cluster component addresses. Short service names: every consumer runs in
// the same namespace, and the umbrella issues the internal certificates with
// only the short names as SANs.
const (
	controllerRegistrationAddr = "https://fsc-open-fsc-controller:9443"
	managerInternalAddr        = "https://fsc-open-fsc-manager-internal:9443"
	managerUnauthAddr          = "https://fsc-open-fsc-manager-internal-unauthenticated:9444"
	auditlogRestAddr           = "https://fsc-open-fsc-auditlog:9443"
	transactionLogAddr         = "https://fsc-open-fsc-txlog-api:9443"
)

// inwayRelease and outwayRelease name the per-gateway Helm releases (and,
// through fullnameOverride, the gateway workloads, services and cert secrets).
// The prefixes double as the orphan-sweep filter, so they must not prefix any
// other release in the namespace.
func inwayRelease(name string) string  { return "fsc-inway-" + name }
func outwayRelease(name string) string { return "fsc-outway-" + name }

func certName(gateway, kind string) string   { return gateway + "-" + kind } // <release>-group / <release>-internal
func certSecret(gateway, kind string) string { return gateway + "-" + kind + "-tls" }

// managerAddress is the address other peers use to reach this installation's
// Manager: the spec override, or the namespaced in-cluster service URL (which
// also serves same-cluster peers joining this installation's group).
func managerAddress(inst *openfscv1.FSCInstallation) string {
	if inst.Spec.ManagerAddress != "" {
		return inst.Spec.ManagerAddress
	}
	return fmt.Sprintf("https://%s.%s:8443", managerExternalService, inst.Namespace)
}

// managerCertDNSNames lists the SANs for the Manager's group certificate in
// Self mode: every name the Manager is reachable under in-cluster, plus the
// host of a spec.managerAddress override.
func managerCertDNSNames(inst *openfscv1.FSCInstallation) []string {
	names := []string{
		managerExternalService,
		fmt.Sprintf("%s.%s", managerExternalService, inst.Namespace),
		fmt.Sprintf("%s.%s.svc.cluster.local", managerExternalService, inst.Namespace),
	}
	if inst.Spec.ManagerAddress != "" {
		if u, err := url.Parse(inst.Spec.ManagerAddress); err == nil && u.Hostname() != "" {
			names = append(names, u.Hostname())
		}
	}
	return names
}
