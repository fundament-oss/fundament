import { After, Given, When, Then } from '@cucumber/cucumber';
import { expect } from '@playwright/test';
import { type Client } from '@connectrpc/connect';
import { ICustomWorld } from '../support/world.ts';
import { createServiceClient } from '../support/api/client.ts';
import { NodePoolsPage } from '../support/pages/node-pools.page.ts';
import { ClusterService } from '../support/generated/v1/cluster_pb.ts';
import { OrganizationService } from '../support/generated/v1/organization_pb.ts';

// The only region the fixtures seed with Gardener's local provider, and the
// newest Kubernetes version they offer in it (db/testdata/031_0101-content.sql).
const REGION = 'local';
const KUBERNETES_VERSION = '1.34.3';
// The pool the cluster is created with, which the scenarios lower or leave.
const EXISTING_POOL = 'workers';

interface QuotaState {
  clusters: Client<typeof ClusterService>;
  organization: Client<typeof OrganizationService>;
  organizationName: string;
  clusterId: string;
  machineType: string;
  /** The pool the scenario adds in the form, which the API may refuse. */
  newPoolName?: string;
  nodePools?: NodePoolsPage;
}

const state = (world: ICustomWorld): QuotaState =>
  world.testData.nodePoolQuota as QuotaState;

const suffix = () => Math.random().toString(36).slice(2, 8);

/** What a new pool of the machine type can still have: the quota less what
 *  every pool of it in the organization takes, read now rather than assumed,
 *  because other clusters in acme-corp use some of it. */
async function roomLeft(
  s: QuotaState,
): Promise<{ quota: number; room: number }> {
  const response = await s.organization.getOrganizationQuotas({});
  const node = response.nodes.find(
    (n) => n.region === REGION && n.machineType === s.machineType,
  );
  const quota = node?.quota ?? 0;
  return { quota, room: quota - (node?.inUse ?? 0) };
}

Given(
  'I have a cluster in region {string} with a {string} node pool with maximum {int}',
  async function (
    this: ICustomWorld,
    region: string,
    machineType: string,
    max: number,
  ) {
    expect(region).toBe(REGION);
    const clusters = createServiceClient(
      ClusterService,
      this.organizationApiUrl!,
      this.authToken,
      this.organizationId,
    );
    const organization = createServiceClient(
      OrganizationService,
      this.organizationApiUrl!,
      this.authToken,
      this.organizationId,
    );
    const { organization: org } = await organization.getOrganization({
      id: this.organizationId!,
    });
    // None is seeded in this region, so the scenario brings its own and takes
    // it down afterwards. Pools are rows in the database here: no Gardener.
    const { clusterId } = await clusters.createCluster({
      name: `e2e-quota-${suffix()}`,
      region,
      kubernetesVersion: KUBERNETES_VERSION,
    });
    this.testData.nodePoolQuota = {
      clusters,
      organization,
      organizationName: org!.name,
      clusterId,
      machineType,
    } satisfies QuotaState;
    await clusters.createNodePool({
      clusterId,
      name: EXISTING_POOL,
      machineType,
      autoscaleMin: 1,
      autoscaleMax: max,
    });
  },
);

Given(
  'the rest of the {string} quota is taken',
  async function (this: ICustomWorld, machineType: string) {
    const s = state(this);
    expect(machineType).toBe(s.machineType);
    const { room } = await roomLeft(s);
    if (room > 0) {
      await s.clusters.createNodePool({
        clusterId: s.clusterId,
        name: `taken-${suffix()}`,
        machineType,
        autoscaleMin: 1,
        autoscaleMax: room,
      });
    }
    expect((await roomLeft(s)).room).toBeLessThanOrEqual(0);
  },
);

When(
  'I open the node pools of that cluster',
  async function (this: ICustomWorld) {
    const s = state(this);
    s.nodePools = new NodePoolsPage(this.page!);
    await s.nodePools.goto(s.organizationName, s.clusterId);
  },
);

When(
  'I add a {string} node pool with a maximum of the room left plus {int}',
  async function (this: ICustomWorld, machineType: string, extra: number) {
    const s = state(this);
    const { room } = await roomLeft(s);
    expect(room).toBeGreaterThan(0);
    const page = s.nodePools!;
    await page.newNodePoolButton.click();
    await page.chooseMachineType(1, machineType);
    s.newPoolName = `over-quota-${suffix()}`;
    await page.setName(1, s.newPoolName);
    await page.setAutoscaleMax(1, room + extra);
    this.testData.room = room;
  },
);

When(
  'another node pool takes {int} {string} node in the meantime',
  async function (this: ICustomWorld, max: number, machineType: string) {
    // Another user, after this form loaded: the form's bound is a hint, and
    // only the API sees this.
    const s = state(this);
    await s.clusters.createNodePool({
      clusterId: s.clusterId,
      name: `meanwhile-${suffix()}`,
      machineType,
      autoscaleMin: 1,
      autoscaleMax: max,
    });
  },
);

When(
  'I lower the maximum of the existing node pool to {int}',
  async function (this: ICustomWorld, max: number) {
    await state(this).nodePools!.setAutoscaleMax(0, max);
  },
);

When('I save the node pools', async function (this: ICustomWorld) {
  await state(this).nodePools!.saveButton.click();
});

Then(
  'the new node pool says how much room is left',
  async function (this: ICustomWorld) {
    const room = this.testData.room as number;
    await expect(state(this).nodePools!.autoscaleMaxHint(1)).toContainText(
      `Up to ${room} for this pool`,
    );
  },
);

Then(
  'the form refuses the maximum before saving',
  async function (this: ICustomWorld) {
    const s = state(this);
    const room = this.testData.room as number;
    await expect(s.nodePools!.autoscaleMaxError(1)).toContainText(
      `At most ${room}`,
    );
    // Still on the sheet: nothing was sent.
    await expect(s.nodePools!.saveButton).toBeVisible();
    await expect(s.nodePools!.errorBanner).toHaveCount(0);
  },
);

Then(
  'I see a refusal that names the {string} quota',
  async function (this: ICustomWorld, machineType: string) {
    const s = state(this);
    const { quota } = await roomLeft(s);
    const text = await s.nodePools!.errorText();
    expect(text).toContain(`machine type ${machineType}`);
    expect(text).toContain(`quota of ${quota}`);
  },
);

Then(
  'the cluster has no node pool by that name',
  async function (this: ICustomWorld) {
    const s = state(this);
    expect(s.newPoolName).toBeTruthy();
    const { nodePools } = await s.clusters.listNodePools({
      clusterId: s.clusterId,
    });
    expect(nodePools.map((pool) => pool.name)).not.toContain(s.newPoolName);
  },
);

Then(
  'the existing node pool has maximum {int}',
  async function (this: ICustomWorld, max: number) {
    const s = state(this);
    // Saving goes back to the cluster's page.
    await this.page!.waitForURL(new RegExp(`/clusters/${s.clusterId}$`), {
      timeout: 10000,
    });
    const { nodePools } = await s.clusters.listNodePools({
      clusterId: s.clusterId,
    });
    expect(
      nodePools.find((pool) => pool.name === EXISTING_POOL)?.maxNodes,
    ).toBe(max);
  },
);

// Pools first, so the quota they take is free at once; a deleted cluster's
// pools count until its shoot is gone.
After({ tags: '@node-pool-quota' }, async function (this: ICustomWorld) {
  const s = this.testData.nodePoolQuota as QuotaState | undefined;
  if (!s) return;
  // Ignore cleanup errors: the scenario's own result is what counts. But a pool
  // that fails to go must not keep the cluster from going too.
  try {
    const { nodePools } = await s.clusters.listNodePools({
      clusterId: s.clusterId,
    });
    await Promise.allSettled(
      nodePools.map((pool) =>
        s.clusters.deleteNodePool({ nodePoolId: pool.id }),
      ),
    );
  } catch {
    // The cluster's deletion below takes its pools along, in time.
  }
  try {
    await s.clusters.deleteCluster({ clusterId: s.clusterId });
  } catch {
    // Nothing left to try.
  }
});
