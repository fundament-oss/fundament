/**
 * Creates the cluster named E2E_CLUSTER_NAME through the API unless it exists,
 * and waits until it runs. `just test-env up` uses it for the baseline cluster
 * the @real-gardener scenarios share.
 *
 *   E2E_CLUSTER_NAME=e2e-base bun scripts/ensure-cluster.ts
 *
 * E2E_CLUSTER_ID_FILE, when set, receives the cluster's id.
 */

import { writeFileSync } from 'node:fs';

import { ClusterService } from '../support/api/cluster-service.ts';
import {
  authenticateWithPassword,
  extractOrganizationId,
  waitForAuthzReady,
} from '../support/auth.ts';
import { waitForClusterRunning } from '../support/cluster-wait.ts';
import { ensureNodePool } from '../support/clusters.ts';

const name = process.env.E2E_CLUSTER_NAME ?? 'e2e-base';
const owner = process.env.E2E_CLUSTER_OWNER_EMAIL ?? 'alice@acme-corp.com';
const region = process.env.E2E_CLUSTER_REGION ?? 'local';
const organizationApiUrl =
  process.env.ORGANIZATION_API_URL ?? 'https://organization.fundament.localhost:8443';
const deadlineMs = 25 * 60_000;

const token = await authenticateWithPassword(owner);
const service = new ClusterService(organizationApiUrl, token, extractOrganizationId(token));

const regions = await waitForAuthzReady(() => service.listRegions());
const offered = regions.find((r) => r.name === region);
if (!offered || offered.kubernetesVersions.length === 0) {
  throw new Error(`region "${region}" offers no Kubernetes version`);
}

let id = (await service.listClusters()).find((c) => c.name === name)?.id;
let changed = false;
if (id) {
  console.log(`cluster ${name} exists (${id})`);
} else {
  const kubernetesVersion = [...offered.kubernetesVersions].sort((a, b) =>
    a.localeCompare(b, undefined, { numeric: true }),
  ).at(-1)!;
  id = await service.createCluster({ name, region, kubernetesVersion });
  changed = true;
  console.log(`created cluster ${name} (${id}, Kubernetes ${kubernetesVersion}) in ${region}`);
}
changed = (await ensureNodePool(service, id)) || changed;

const started = Date.now();
const cluster = await waitForClusterRunning(service, id, deadlineMs, changed);
console.log(
  `cluster ${name} runs after ${Math.round((Date.now() - started) / 1000)}s (status ${cluster.status})`,
);
if (process.env.E2E_CLUSTER_ID_FILE) {
  writeFileSync(process.env.E2E_CLUSTER_ID_FILE, id);
}
