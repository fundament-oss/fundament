package controller

import (
	openfscv1 "github.com/fundament-oss/fundament/openfsc-operator/api/v1"
)

// coreValues renders the values for the OpenFSC umbrella release of an
// installation. The umbrella ships only the internal mTLS chain; the group
// (federation) trust is wired here per directory mode: Self points every
// component at the operator-minted group CA and Manager certificate, External
// at the Secrets the team provided.
func coreValues(inst *openfscv1.FSCInstallation) map[string]any {
	groupTrustName, groupTrustKey := groupCASecret, "tls.crt"
	directoryAddress := managerAddress(inst)
	directoryPeerID := inst.Spec.PeerID
	managerGroupSecret := managerGroupCertSecret
	if ext := inst.Spec.Directory.External; ext != nil {
		groupTrustName, groupTrustKey = ext.TrustAnchor.Name, ext.TrustAnchor.Key
		directoryAddress = ext.Address
		directoryPeerID = ext.PeerID
		managerGroupSecret = inst.Spec.Certificate.ExistingSecret
	}
	groupCertRef := func() map[string]any {
		return map[string]any{"existingSecret": managerGroupSecret}
	}

	return map[string]any{
		"fullnameOverride": umbrellaRelease,
		"global":           map[string]any{"groupID": inst.Spec.GroupID},
		// The operator installs gateways itself, one release per declared
		// inway/outway, so the umbrella's bundled pair is disabled.
		"open-fsc-inway":  map[string]any{"enabled": false},
		"open-fsc-outway": map[string]any{"enabled": false},
		"open-fsc-manager": map[string]any{
			"config": map[string]any{
				"selfAddress":                      managerAddress(inst),
				"directoryManagerAddress":          directoryAddress,
				"directoryPeerID":                  directoryPeerID,
				"controllerRegistrationApiAddress": controllerRegistrationAddr,
				"autoSignGrants":                   stringsToAny(inst.Spec.AutoSignGrants),
			},
			"postgresql": postgresValues("open_fsc_manager"),
			"certificates": map[string]any{
				"internal":                internalCertValues(managerInternalSecret),
				"internalUnauthenticated": internalCertValues(managerUnauthSecret),
				"group": map[string]any{
					"caCertificatePEMExistingSecret": map[string]any{
						"name": groupTrustName,
						"key":  groupTrustKey,
					},
					// The Manager presents one group certificate for its peer,
					// token and signature identities.
					"peer":      groupCertRef(),
					"token":     groupCertRef(),
					"signature": groupCertRef(),
				},
			},
		},
		"open-fsc-controller": map[string]any{
			"config": map[string]any{
				"managerInternalAddress": managerInternalAddr,
				"auditlog": map[string]any{
					"type": "rest",
					"rest": map[string]any{"address": auditlogRestAddr},
				},
			},
			// No ingress for the Controller UI (the umbrella enables it by
			// default); access goes through spec.controllerURL instead.
			"ingress":    map[string]any{"enabled": false},
			"postgresql": postgresValues("open_fsc_controller"),
			"certificates": map[string]any{
				"internal": internalCertValues(controllerInternalSecret),
			},
		},
		"open-fsc-auditlog": map[string]any{
			"db": postgresValues("open_fsc_auditlog"),
			"certificates": map[string]any{
				"internal": internalCertValues(auditlogInternalSecret),
			},
		},
		"open-fsc-txlog-api": map[string]any{
			"txlogdb": postgresValues("open_fsc_tx_log"),
			"certificates": map[string]any{
				"internal": internalCertValues(txlogInternalSecret),
			},
		},
	}
}

func postgresValues(database string) map[string]any {
	return map[string]any{
		"hostname": postgresService,
		"database": database,
		"existingSecret": map[string]any{
			"name":        postgresSecret,
			"usernameKey": "username",
			"passwordKey": "password",
		},
	}
}

func internalCertValues(secret string) map[string]any {
	return map[string]any{
		"existingSecret": secret,
		"caCertificatePEMExistingSecret": map[string]any{
			"name": internalCASecret,
			"key":  "tls.crt",
		},
	}
}

func stringsToAny(in []string) []any {
	out := make([]any, len(in))
	for i, s := range in {
		out[i] = s
	}
	return out
}
