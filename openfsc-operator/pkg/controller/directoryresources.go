package controller

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openfscv1 "github.com/fundament-oss/fundament/openfsc-operator/pkg/api/v1"
)

// fieldOwner is the server-side-apply field manager for resources the operator
// owns directly (not through Helm).
const fieldOwner = "openfsc-operator"

// Directory prerequisite resource names in the directory namespace. The
// open-fsc umbrella ships only the internal mTLS CA, so the group (federation)
// CA, the Manager's group certificate and a Postgres cluster — all of which a
// self-contained directory peer needs — are provided here. In OpenFSC a peer is
// identified by its certificate's subject serialNumber, so the group CA itself
// can be self-signed (this is a self-contained, single-peer directory with no
// federation partners to share a CA with).
const (
	groupSelfSignedIssuer = "shared-group-selfsigned"
	// managerExternalName is the in-cluster service name (and group certificate
	// SAN) of the directory Manager's external API. It must match the umbrella's
	// manager-external service: "<release>-open-fsc-manager-external".
	managerExternalName = "shared-open-fsc-manager-external"
	managerGroupCert    = "shared-directory-manager-external-tls"
	postgresCluster     = "shared-postgresql"
)

// directoryResources renders the prerequisite objects for the given Directory:
// the self-signed group CA chain (Issuer + CA Certificate + group Issuer), the
// Manager's group certificate carrying the directory peer ID, and the
// CloudNativePG cluster the umbrella's components share. CNPG creates the
// "shared-postgresql-rw" service and the "shared-postgresql-app" secret the
// umbrella override references.
func directoryResources(dir *openfscv1.Directory) []*unstructured.Unstructured {
	ns := dir.Spec.Namespace

	selfSigned := newUnstructured("cert-manager.io/v1", "Issuer", ns, groupSelfSignedIssuer, map[string]any{
		"selfSigned": map[string]any{},
	})

	caCert := newUnstructured("cert-manager.io/v1", "Certificate", ns, groupCASecret, map[string]any{
		"isCA":        true,
		"commonName":  "OpenFSC Directory Group CA",
		"secretName":  groupCASecret,
		"privateKey":  map[string]any{"algorithm": "RSA", "size": int64(4096)},
		"duration":    "87600h", // 10 years
		"renewBefore": "8760h",  // 1 year
		"issuerRef":   map[string]any{"name": groupSelfSignedIssuer, "kind": "Issuer"},
	})

	// The group Issuer. The gateway reconcilers reference it by name ("shared")
	// when issuing inway/outway group certificates.
	caIssuer := newUnstructured("cert-manager.io/v1", "Issuer", ns, groupIssuer, map[string]any{
		"ca": map[string]any{"secretName": groupCASecret},
	})

	// The directory Manager's group certificate (peer/token/signature identity).
	// The subject serialNumber is the directory peer ID; the SAN is the
	// manager-external service name so other peers (and the Manager itself,
	// acting as the Directory) can verify it.
	managerCert := newUnstructured("cert-manager.io/v1", "Certificate", ns, managerGroupCert, map[string]any{
		"secretName": managerGroupCert,
		"commonName": managerExternalName,
		"dnsNames":   []any{managerExternalName},
		"subject": map[string]any{
			"organizations": []any{peerOrg},
			"serialNumber":  dir.Spec.PeerID,
		},
		"privateKey":  map[string]any{"size": int64(4096)},
		"duration":    "8760h", // 1 year
		"renewBefore": "720h",  // 30 days
		"issuerRef":   map[string]any{"name": groupIssuer, "kind": "Issuer"},
	})

	// CRD defaulting normally fills these (postgres defaults to {}), but guard
	// against objects created before defaulting, e.g. from older manifests.
	pg := dir.Spec.Postgres
	if pg.Instances <= 0 {
		pg.Instances = 1
	}
	if pg.Image == "" {
		pg.Image = "ghcr.io/cloudnative-pg/postgresql:16"
	}
	if pg.StorageClass == "" {
		pg.StorageClass = "basic-csi"
	}
	if pg.StorageSize == "" {
		pg.StorageSize = "1Gi"
	}
	postgres := newUnstructured("postgresql.cnpg.io/v1", "Cluster", ns, postgresCluster, map[string]any{
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

	return []*unstructured.Unstructured{selfSigned, caCert, caIssuer, managerCert, postgres}
}

func newUnstructured(apiVersion, kind, ns, name string, spec map[string]any) *unstructured.Unstructured {
	u := &unstructured.Unstructured{Object: map[string]any{"spec": spec}}
	u.SetAPIVersion(apiVersion)
	u.SetKind(kind)
	u.SetNamespace(ns)
	u.SetName(name)
	return u
}

// ensureNamespace creates the directory namespace if it does not exist.
func ensureNamespace(ctx context.Context, c client.Client, name string) error {
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
	if err := c.Create(ctx, ns); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create namespace %s: %w", name, err)
	}
	return nil
}

// applyDirectoryResources server-side-applies the directory prerequisites.
func applyDirectoryResources(ctx context.Context, c client.Client, dir *openfscv1.Directory) error {
	for _, obj := range directoryResources(dir) {
		if err := c.Apply(ctx, client.ApplyConfigurationFromUnstructured(obj), client.ForceOwnership, client.FieldOwner(fieldOwner)); err != nil {
			return fmt.Errorf("apply %s %s: %w", obj.GetKind(), obj.GetName(), err)
		}
	}
	return nil
}

// deleteDirectoryResources removes the SSA-applied prerequisites. Missing
// resources — or missing CRDs, when the prerequisite operators were already
// uninstalled — are not errors.
func deleteDirectoryResources(ctx context.Context, c client.Client, dir *openfscv1.Directory) error {
	for _, obj := range directoryResources(dir) {
		err := c.Delete(ctx, obj)
		if err == nil || apierrors.IsNotFound(err) || meta.IsNoMatchError(err) {
			continue
		}
		return fmt.Errorf("delete %s %s: %w", obj.GetKind(), obj.GetName(), err)
	}
	return nil
}
