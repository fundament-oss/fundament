package controller

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openfscv1 "github.com/fundament-oss/fundament/openfsc-operator/api/v1"
)

// fieldOwner is the server-side-apply field manager for resources the operator
// owns directly (not through Helm).
const fieldOwner = "openfsc-operator"

// coreResources renders the objects the operator provisions next to the
// umbrella release: always the CloudNativePG cluster backing the components,
// and in Self mode the self-signed group (federation) CA chain plus the
// Manager's group certificate. The umbrella ships only the internal mTLS CA;
// in OpenFSC a peer is identified by its certificate's subject serialNumber,
// so a self-contained group can run on a self-signed CA.
func coreResources(inst *openfscv1.FSCInstallation) []*unstructured.Unstructured {
	resources := []*unstructured.Unstructured{postgresResource(inst)}
	if inst.Spec.Directory.Mode == openfscv1.DirectoryModeSelf {
		resources = append(resources, groupCAResources(inst)...)
	}
	return resources
}

func groupCAResources(inst *openfscv1.FSCInstallation) []*unstructured.Unstructured {
	ns := inst.Namespace

	selfSigned := newUnstructured("cert-manager.io/v1", "Issuer", ns, groupSelfSignedIssuer, map[string]any{
		"selfSigned": map[string]any{},
	})

	caCert := newUnstructured("cert-manager.io/v1", "Certificate", ns, groupCASecret, map[string]any{
		"isCA":       true,
		"commonName": "OpenFSC Group CA " + inst.Spec.GroupID,
		// FSC validates that a presented cert's issuer carries an organization;
		// cert-manager copies this subject DN into the Issuer of every leaf the
		// CA signs, so it must match the leaf subject organization.
		"subject":     map[string]any{"organizations": []any{peerOrganization(inst)}},
		"secretName":  groupCASecret,
		"privateKey":  map[string]any{"algorithm": "RSA", "size": int64(4096)},
		"duration":    "87600h", // 10 years
		"renewBefore": "8760h",  // 1 year
		"issuerRef":   map[string]any{"name": groupSelfSignedIssuer, "kind": "Issuer"},
	})

	// The gateway certificates are issued from this Issuer too.
	caIssuer := newUnstructured("cert-manager.io/v1", "Issuer", ns, groupIssuer, map[string]any{
		"ca": map[string]any{"secretName": groupCASecret},
	})

	// The Manager's group certificate (peer/token/signature identity). The
	// subject serialNumber is the peer ID; the SANs cover every name the
	// Manager is reachable under, so peers in other namespaces can verify it.
	managerCert := newUnstructured("cert-manager.io/v1", "Certificate", ns, managerGroupCertSecret, map[string]any{
		"secretName": managerGroupCertSecret,
		"commonName": managerExternalService,
		"dnsNames":   stringsToAny(managerCertDNSNames(inst)),
		"subject": map[string]any{
			"organizations": []any{peerOrganization(inst)},
			"serialNumber":  inst.Spec.PeerID,
		},
		"privateKey":  map[string]any{"size": int64(4096)},
		"duration":    "8760h", // 1 year
		"renewBefore": "720h",  // 30 days
		"issuerRef":   map[string]any{"name": groupIssuer, "kind": "Issuer"},
	})

	return []*unstructured.Unstructured{selfSigned, caCert, caIssuer, managerCert}
}

func postgresResource(inst *openfscv1.FSCInstallation) *unstructured.Unstructured {
	pg := inst.Spec.Postgres
	return newUnstructured("postgresql.cnpg.io/v1", "Cluster", inst.Namespace, postgresCluster, map[string]any{
		"instances": int64(pg.Instances),
		"imageName": pg.Image,
		"storage": map[string]any{
			"size":         pg.StorageSize,
			"storageClass": pg.StorageClass,
		},
		"bootstrap": map[string]any{
			"initdb": map[string]any{
				"database": "app",
				"owner":    "app",
				// One database per component: the OpenFSC images ignore the
				// x-migrations-table DSN param, so every component would otherwise
				// share a single public.schema_migrations table on a shared database
				// and corrupt each other's migration state.
				"postInitSQL": []any{
					"CREATE DATABASE open_fsc_manager WITH OWNER app;",
					"CREATE DATABASE open_fsc_controller WITH OWNER app;",
					"CREATE DATABASE open_fsc_auditlog WITH OWNER app;",
					"CREATE DATABASE open_fsc_tx_log WITH OWNER app;",
				},
			},
		},
	})
}

func newUnstructured(apiVersion, kind, ns, name string, spec map[string]any) *unstructured.Unstructured {
	u := &unstructured.Unstructured{Object: map[string]any{"spec": spec}}
	u.SetAPIVersion(apiVersion)
	u.SetKind(kind)
	u.SetNamespace(ns)
	u.SetName(name)
	return u
}

// applyCoreResources server-side-applies the installation's core resources.
func applyCoreResources(ctx context.Context, c client.Client, inst *openfscv1.FSCInstallation) error {
	for _, obj := range coreResources(inst) {
		if err := c.Apply(ctx, client.ApplyConfigurationFromUnstructured(obj), client.ForceOwnership, client.FieldOwner(fieldOwner)); err != nil {
			return fmt.Errorf("apply %s %s: %w", obj.GetKind(), obj.GetName(), err)
		}
	}
	return nil
}

// deleteCoreResources removes the SSA-applied core resources. Missing
// resources — or missing CRDs, when the prerequisite operators were already
// uninstalled — are not errors.
func deleteCoreResources(ctx context.Context, c client.Client, inst *openfscv1.FSCInstallation) error {
	for _, obj := range coreResources(inst) {
		err := c.Delete(ctx, obj)
		if err == nil || apierrors.IsNotFound(err) || meta.IsNoMatchError(err) {
			continue
		}
		return fmt.Errorf("delete %s %s: %w", obj.GetKind(), obj.GetName(), err)
	}
	return nil
}
