package controller

import (
	"fmt"

	openfscv1 "github.com/fundament-oss/fundament/openfsc-operator/api/v1"
)

// inwayValues renders the values for one open-fsc-inway release (--set
// semantics).
func inwayValues(inst *openfscv1.FSCInstallation, gw openfscv1.InwayConfig) map[string]string {
	release := inwayRelease(gw.Name)
	v := gatewayCommonValues(inst, release, gw.Name, gw.Certificate)
	v["service.type"] = "ClusterIP"
	v["service.port"] = "443" // the inway's registered selfAddress must use :443
	v["config.selfAddress"] = inwaySelfAddress(inst.Namespace, gw)
	v["config.managerInternalUnauthenticatedAddress"] = managerUnauthAddr
	return v
}

// inwaySelfAddress is the address the inway registers for other peers to reach
// it: the spec override, or the namespaced in-cluster service URL.
func inwaySelfAddress(ns string, gw openfscv1.InwayConfig) string {
	if gw.SelfAddress != "" {
		return gw.SelfAddress
	}
	return fmt.Sprintf("https://%s.%s:443", inwayRelease(gw.Name), ns)
}

// outwayValues renders the values for one open-fsc-outway release. An outway
// is a forward proxy: no selfAddress and no inbound server cert.
func outwayValues(inst *openfscv1.FSCInstallation, gw openfscv1.OutwayConfig) map[string]string {
	release := outwayRelease(gw.Name)
	v := gatewayCommonValues(inst, release, gw.Name, gw.Certificate)
	v["https.enabled"] = "false"
	v["service.type"] = "ClusterIP"
	v["config.managerInternalAddress"] = managerInternalAddr
	return v
}

// outwayURL is the in-cluster consume endpoint of an outway, surfaced in the
// gateway status.
func outwayURL(ns string, gw openfscv1.OutwayConfig) string {
	return fmt.Sprintf("http://%s.%s:80", outwayRelease(gw.Name), ns)
}

// gatewayCommonValues wires a gateway to the installation's OpenFSC core. The
// gateway's group certificate is the per-namespace minted one in Self mode; in
// External mode it is the installation certificate, unless the gateway
// declares its own.
func gatewayCommonValues(inst *openfscv1.FSCInstallation, release, fscName string, cert *openfscv1.CertificateRef) map[string]string {
	groupTrustName, groupTrustKey := groupCASecret, "tls.crt"
	groupSecret := certSecret(release, "group")
	if ext := inst.Spec.Directory.External; ext != nil {
		groupTrustName, groupTrustKey = ext.TrustAnchor.Name, ext.TrustAnchor.Key
		groupSecret = inst.Spec.Certificate.ExistingSecret
		if cert != nil {
			groupSecret = cert.ExistingSecret
		}
	}
	return map[string]string{
		"fullnameOverride": release,
		"image.pullPolicy": "IfNotPresent",
		"global.groupID":   inst.Spec.GroupID,
		"global.certificates.group.caCertificatePEMExistingSecret.name":    groupTrustName,
		"global.certificates.group.caCertificatePEMExistingSecret.key":     groupTrustKey,
		"global.certificates.internal.caCertificatePEMExistingSecret.name": internalCASecret,
		"global.certificates.internal.caCertificatePEMExistingSecret.key":  "tls.crt",
		"config.name": fscName,
		"config.controllerRegistrationApiAddress": controllerRegistrationAddr,
		"config.transactionLogApiAddress":         transactionLogAddr,
		"config.disableCrlChecks":                 "true",
		"certificates.group.existingSecret":       groupSecret,
		"certificates.internal.existingSecret":    certSecret(release, "internal"),
	}
}
