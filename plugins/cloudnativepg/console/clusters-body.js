// The Cluster the create form submits, and the rules for its inputs. Pure: no
// DOM, no SDK. test-resources.yaml carries the same shape; keep them in step.

import { parseQuantity } from './quantity.js';

export const CLUSTER_RESOURCE = { group: 'postgresql.cnpg.io', version: 'v1', resource: 'clusters' };

// The ClusterImageCatalog the plugin keeps (cnpg/catalog.go): the current,
// digest-pinned image per PostgreSQL major. A database names only its major,
// so it moves to a new minor, with a restart, when a plugin upgrade raises
// the catalog.
const IMAGE_CATALOG = 'fundament-postgresql';

// Operator 1.26 runs PostgreSQL 13 to 17; 18 needs operator 1.27. First is the
// default.
export const POSTGRES_MAJORS = [
  { major: 17, label: 'PostgreSQL 17' },
  { major: 16, label: 'PostgreSQL 16' },
];

// Room to grow: growing a volume takes an organization admin.
export const DEFAULT_STORAGE_SIZE = '5Gi';

// Explicit, so a small LimitRange default in the namespace cannot starve or
// OOM-kill Postgres. No CPU limit: throttling a database hurts more than it
// protects.
const RESOURCES = {
  requests: { cpu: '100m', memory: '256Mi' },
  limits: { memory: '1Gi' },
};

// CNPG names Services (<name>-rw, <name>-ro, <name>-r) and Secrets after the
// Cluster, so the name must be a DNS-1035 label: it starts with a letter. The
// cap leaves room for those suffixes.
const NAME_MAX_LENGTH = 50;

export function clusterNameError(name) {
  if (!name) return 'Please enter a name.';
  if (!/^[a-z]([a-z0-9-]*[a-z0-9])?$/.test(name)) {
    return 'Start with a letter and use lowercase letters, digits and dashes only.';
  }
  if (name.length > NAME_MAX_LENGTH) return `Use at most ${NAME_MAX_LENGTH} characters.`;
  return null;
}

export function namespaceError(namespace) {
  if (!namespace) return 'Please choose a namespace.';
  if (!/^[a-z0-9]([a-z0-9-]*[a-z0-9])?$/.test(namespace) || namespace.length > 63) {
    return 'Enter a valid namespace.';
  }
  return null;
}

// A floor, not a sizing rule: the one volume holds the data and the WAL, which
// PostgreSQL lets grow toward max_wal_size (1GB) between checkpoints. Below
// 1Gi a database can fail to start or fill its disk at once, and one stuck
// that way cannot be deleted from the console.
const MIN_STORAGE_BYTES = 2 ** 30;
// A single-instance database without backups; anything near this size wants
// an organization admin's attention anyway. It also keeps the request inside
// what a provisioner accepts: rbd refuses a size near 2^63 bytes, and the
// database then waits for its volume for good.
const MAX_STORAGE_BYTES = 2 ** 40;

export function storageSizeError(size) {
  const bytes = parseQuantity(size);
  if (bytes === null) return 'Enter a size like 10Gi, 500Mi or 20G.';
  if (bytes < MIN_STORAGE_BYTES) return 'Use at least 1Gi.';
  if (bytes > MAX_STORAGE_BYTES) return 'Use at most 1Ti.';
  return null;
}

// Empty means the cluster default, so it is an error only when the cluster has
// none: the PVC would stay Pending, and the database cannot be deleted from
// the console to try again.
export function storageClassError(storageClass, { required }) {
  if (!storageClass) return required ? 'Please choose a StorageClass.' : null;
  if (!isDnsSubdomain(storageClass)) return 'Enter a valid StorageClass name.';
  return null;
}

// A DNS-1123 subdomain, the rule for StorageClass names: dot-separated labels
// of at most 63 characters each, 253 in all.
function isDnsSubdomain(value) {
  return (
    value.length <= 253 &&
    value.split('.').every((label) => label.length <= 63 && /^[a-z0-9]([a-z0-9-]*[a-z0-9])?$/.test(label))
  );
}

// No bootstrap block: CNPG's default initdb creates database "app" owned by
// user "app", with the credentials in Secret <name>-app. No backup, no
// postgresql.parameters, no extensions: out of scope for this version.
export function buildCluster({ name, namespace, major, size, storageClass }) {
  return {
    apiVersion: 'postgresql.cnpg.io/v1',
    kind: 'Cluster',
    metadata: {
      name,
      namespace,
      labels: { 'app.kubernetes.io/managed-by': 'fundament-cloudnativepg' },
    },
    spec: {
      instances: 1,
      imageCatalogRef: {
        apiGroup: 'postgresql.cnpg.io',
        kind: 'ClusterImageCatalog',
        name: IMAGE_CATALOG,
        major,
      },
      // With one instance, CNPG's PodDisruptionBudget allows no disruption at
      // all, which blocks every node drain during node pool rollouts and
      // maintenance. The database is down while its node drains either way.
      enablePDB: false,
      resources: RESOURCES,
      // No storageClass means the cluster's default StorageClass.
      storage: { size, ...(storageClass ? { storageClass } : {}) },
    },
  };
}
