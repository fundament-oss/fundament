/**
 * Steps for the @cluster-lifecycle feature: a cluster created through the API
 * becomes a shoot in the real Gardener, serves through its kubeconfig, and is
 * gone after deletion. Runs in the test-env VM (`just test-env test`).
 */

import { When, Then, After } from '@cucumber/cucumber';
import { expect } from '@playwright/test';
import { ICustomWorld } from '../support/world.ts';
import { ClusterService } from '../support/api/cluster-service.ts';
import { waitForClusterDeleted, waitForClusterRunning } from '../support/cluster-wait.ts';
import { ensureNodePool } from '../support/clusters.ts';
import { pollUntil } from '../support/poll.ts';
import { withClusterAccess } from '../support/kube.ts';
import { shootForCluster } from '../support/shoot.ts';

function clusterService(world: ICustomWorld): ClusterService {
  world.clusterService ??= new ClusterService(
    world.organizationApiUrl!,
    world.authToken!,
    world.organizationId,
  );
  return world.clusterService;
}

// Delete the cluster when the scenario stopped before its own delete step, so a
// failed run does not leave a shoot taking memory in the seed.
After({ tags: '@cluster-lifecycle', timeout: 60_000 }, async function (this: ICustomWorld) {
  const id = this.testData.lifecycleClusterId as string | undefined;
  if (!id || this.testData.lifecycleClusterDeleted) {
    return;
  }
  try {
    await clusterService(this).deleteCluster(id);
  } catch (err) {
    console.error(`cleanup: failed to delete cluster ${id}`, err);
  }
});

When(
  'I create a cluster in region {string}',
  { timeout: 60_000 },
  async function (this: ICustomWorld, region: string) {
    const service = clusterService(this);
    const offered = (await service.listRegions()).find((r) => r.name === region);
    expect(offered?.kubernetesVersions.length, `region "${region}" offers no Kubernetes version`).toBeGreaterThan(0);
    const kubernetesVersion = [...offered!.kubernetesVersions]
      .sort((a, b) => a.localeCompare(b, undefined, { numeric: true }))
      .at(-1)!;
    const name = `e2e-lc-${Date.now().toString(36)}`;
    const id = await service.createCluster({ name, region, kubernetesVersion });
    // Recorded before the pool, so the After hook deletes the cluster if that fails.
    this.testData.lifecycleClusterId = id;
    this.testData.lifecycleClusterName = name;
    await ensureNodePool(service, id);
  },
);

// No cucumber timeout: the wait enforces the minutes the feature names.
Then(
  'within {int} minutes the cluster is running',
  { timeout: -1 },
  async function (this: ICustomWorld, minutes: number) {
    await waitForClusterRunning(
      clusterService(this),
      this.testData.lifecycleClusterId as string,
      minutes * 60_000,
      true,
    );
  },
);

Then(
  'its Gardener shoot was created successfully',
  { timeout: 30_000 },
  async function (this: ICustomWorld) {
    const shoot = shootForCluster(this.testData.lifecycleClusterId as string);
    expect(shoot, 'no shoot carries the cluster-id label').toBeDefined();
    expect(shoot!.status?.lastOperation?.state).toBe('Succeeded');
  },
);

Then(
  "the cluster's kubeconfig lists the namespace {string}",
  { timeout: 120_000 },
  async function (this: ICustomWorld, namespace: string) {
    const session = { organizationApiUrl: this.organizationApiUrl!, authToken: this.authToken!, organizationId: this.organizationId };
    await withClusterAccess(session, this.testData.lifecycleClusterId as string, (kubectl) =>
      // The kube-api-proxy learns about a new cluster a little after it runs.
      pollUntil(60_000, `namespace ${namespace} through the cluster's kubeconfig`, async () => {
        try {
          return kubectl(['get', 'namespace', namespace]);
        } catch {
          return undefined;
        }
      }),
    );
  },
);

When('I delete the cluster', { timeout: 30_000 }, async function (this: ICustomWorld) {
  await clusterService(this).deleteCluster(this.testData.lifecycleClusterId as string);
  this.testData.lifecycleClusterDeleted = true;
});

Then(
  'within {int} minutes the cluster is no longer listed',
  { timeout: -1 },
  async function (this: ICustomWorld, minutes: number) {
    await waitForClusterDeleted(
      clusterService(this),
      this.testData.lifecycleClusterId as string,
      minutes * 60_000,
    );
  },
);

Then('its Gardener shoot is gone', { timeout: 30_000 }, async function (this: ICustomWorld) {
  expect(shootForCluster(this.testData.lifecycleClusterId as string)).toBeUndefined();
});
