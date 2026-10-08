// The Cluster the create form submits, and the rules for its inputs. Pure: no
// DOM, no SDK. test-resources.yaml carries the same shape; keep them in step.

export const CLUSTER_RESOURCE = { group: 'postgresql.cnpg.io', version: 'v1', resource: 'clusters' };

// Operator 1.26 runs PostgreSQL 13 to 17; 18 needs operator 1.27. First is the
// default. Exact minor tags, so a database never changes version on its own.
export const POSTGRES_IMAGES = [
  { image: 'ghcr.io/cloudnative-pg/postgresql:17.4', label: 'PostgreSQL 17.4' },
  { image: 'ghcr.io/cloudnative-pg/postgresql:16.8', label: 'PostgreSQL 16.8' },
];

export const DEFAULT_STORAGE_SIZE = '1Gi';

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
const MIN_STORAGE_MI = 1024;
const UNIT_MI = { Mi: 1, Gi: 1024, Ti: 1024 * 1024 };

export function storageSizeError(size) {
  const match = /^([1-9][0-9]*)(Mi|Gi|Ti)$/.exec(size);
  if (!match) {
    return 'Enter the storage size as a whole number followed by Mi, Gi or Ti, for example 1Gi.';
  }
  if (Number(match[1]) * UNIT_MI[match[2]] < MIN_STORAGE_MI) return 'Use at least 1Gi.';
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
export function buildCluster({ name, namespace, imageName, size, storageClass }) {
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
      imageName,
      resources: RESOURCES,
      // No storageClass means the cluster's default StorageClass.
      storage: { size, ...(storageClass ? { storageClass } : {}) },
    },
  };
}
