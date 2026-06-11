package controller

import (
	"fmt"
)

// OpenFSC component service endpoints in the directory namespace (release
// "shared", fullnameOverride=shared). The inway/outway workloads run in the
// same namespace as the directory's controller/manager, so short in-cluster
// service names resolve and the umbrella's cert-manager issuers can sign their
// certs.
const (
	controllerRegistrationAddr = "https://shared-open-fsc-controller:9443"
	managerInternalAddr        = "https://shared-open-fsc-manager-internal:9443"
	managerUnauthAddr          = "https://shared-open-fsc-manager-internal-unauthenticated:9444"
	transactionLogAddr         = "https://shared-open-fsc-txlog-api:9443"
)

// inwayValues renders the values for the open-fsc-inway chart (--set
// semantics). gateway is the k8s release/DNS name; fscName is the FSC name the
// inway registers under.
func inwayValues(ns, gateway, fscName, groupID string) map[string]string {
	v := gatewayCommonValues(gateway, fscName, groupID)
	v["service.type"] = "ClusterIP"
	v["service.port"] = "443" // inway requires its registered selfAddress on :443
	v["config.selfAddress"] = fmt.Sprintf("https://%s.%s:443", gateway, ns)
	v["config.managerInternalUnauthenticatedAddress"] = managerUnauthAddr
	return v
}

// outwayValues renders the values for the open-fsc-outway chart. An outway is a
// forward proxy: no selfAddress and no inbound server cert.
func outwayValues(gateway, fscName, groupID string) map[string]string {
	v := gatewayCommonValues(gateway, fscName, groupID)
	v["https.enabled"] = "false"
	v["service.type"] = "ClusterIP"
	v["config.managerInternalAddress"] = managerInternalAddr
	return v
}

func gatewayCommonValues(gateway, fscName, groupID string) map[string]string {
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
