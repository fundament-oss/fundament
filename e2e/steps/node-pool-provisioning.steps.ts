/**
 * Steps for the @real-gardener node pool provisioning feature.
 *
 * This feature only passes against a real Gardener seed (not the mock used by
 * PR previews): it cordons/uncordons an actual kind node so the shoot's
 * machine-controller-manager genuinely cannot place machines, then watches
 * ClusterService report that through the API. The default cucumber profile
 * excludes @real-gardener (see cucumber.json); run it explicitly with
 * `just e2e test-real-gardener` against `just dev -p local-gardener`.
 */

import { execFileSync } from 'node:child_process';
import { Given, When, Then, Before, After } from '@cucumber/cucumber';
import { expect } from '@playwright/test';
import { ICustomWorld } from '../support/world.ts';
import { ClusterService, type NodePool } from '../support/api/cluster-service.ts';
import { ConnectRpcError } from '../support/api/client.ts';
import { ClusterStatus, NodePoolStatus } from '../support/generated/v1/common_pb.ts';
import { authenticateWithPassword, extractOrganizationId } from './common.steps.ts';

// The scenario always targets this cluster/pool; the pool name is also
// hardcoded (not read from world state) so the After hook can sweep it even
// if a prior failed run never got far enough to record it.
const clusterName = process.env.E2E_CLUSTER_NAME ?? 'tf-a';
const clusterOwnerEmail = process.env.E2E_CLUSTER_OWNER_EMAIL ?? 'alice@acme-corp.com';
const machineType = process.env.E2E_NODE_POOL_MACHINE_TYPE ?? 'local-small';
const NODE_POOL_NAME = 'nomach';

const seedContext = process.env.E2E_SEED_CONTEXT ?? 'kind-gardener-operator-local';
const seedNode = process.env.E2E_SEED_NODE ?? 'gardener-operator-local-control-plane';

const POLL_INTERVAL_MS = 15_000;

// Authz propagation tolerance: mirrors waitForAuthzReady in common.steps.ts.
// Against a freshly-deployed local-gardener environment, a user can
// authenticate before the authz-worker has written their org-membership
// tuples to OpenFGA, so the first permission-gated call can transiently fail
// with permission_denied. Retrying here (rather than a separate throwaway
// probe call) matters a lot more than in common.steps.ts: a flake that costs
// a 5xx there costs a 10-20 minute re-run here.
const AUTHZ_READY_TIMEOUT_MS = 60_000;
const AUTHZ_READY_INTERVAL_MS = 2_000;

async function waitForAuthzReady<T>(probe: () => Promise<T>): Promise<T> {
  const deadline = Date.now() + AUTHZ_READY_TIMEOUT_MS;
  for (;;) {
    try {
      return await probe();
    } catch (err) {
      if (
        err instanceof ConnectRpcError &&
        err.code === 'permission_denied' &&
        Date.now() < deadline
      ) {
        await new Promise((resolve) =>
          setTimeout(resolve, AUTHZ_READY_INTERVAL_MS),
        );
        continue;
      }
      throw err;
    }
  }
}

function cordonSeed(on: boolean) {
  execFileSync(
    'kubectl',
    ['--context', seedContext, on ? 'cordon' : 'uncordon', seedNode],
    { stdio: 'inherit' },
  );
}

/**
 * Poll `check` every POLL_INTERVAL_MS until it returns a value, or throw once
 * `deadlineMs` has elapsed. `check` returns `undefined` to keep polling.
 */
async function pollUntil<T>(
  deadlineMs: number,
  description: string,
  check: () => Promise<T | undefined>,
): Promise<T> {
  const deadline = Date.now() + deadlineMs;
  for (;;) {
    const result = await check();
    if (result !== undefined) {
      return result;
    }
    if (Date.now() >= deadline) {
      throw new Error(
        `Timed out after ${Math.round(deadlineMs / 1000)}s waiting for: ${description}`,
      );
    }
    await new Promise((resolve) => setTimeout(resolve, POLL_INTERVAL_MS));
  }
}

function findNodePool(pools: NodePool[], name: string): NodePool | undefined {
  return pools.find((pool) => pool.name === name);
}

/**
 * The gherkin's cluster/node-pool status strings ("UNHEALTHY", "RUNNING", ...)
 * are exactly the enum member names, so look them up directly rather than
 * hand-maintaining a parallel string -> enum map.
 */
function clusterStatusByName(name: string): ClusterStatus {
  const value = ClusterStatus[name as keyof typeof ClusterStatus];
  if (value === undefined) {
    throw new Error(`unknown cluster status "${name}"`);
  }
  return value;
}

// --- Hooks ---

// This feature doesn't go through the @api Before hook (it isn't tagged
// @api), so set up the organization-api URL state it needs itself.
Before({ tags: '@real-gardener', timeout: 10_000 }, async function (
  this: ICustomWorld,
) {
  this.organizationApiUrl =
    process.env.ORGANIZATION_API_URL ||
    'https://organization.fundament.localhost:8443';
  this.createdNodePoolIds = new Map();
  this.seedCordoned = false;
});

// Always uncordon the seed and delete the "nomach" pool, even on failure, so
// a failed run doesn't poison the next one.
After({ tags: '@real-gardener', timeout: 60_000 }, async function (
  this: ICustomWorld,
) {
  try {
    cordonSeed(false);
  } catch (err) {
    console.error('cleanup: failed to uncordon seed node', err);
  }

  if (!this.clusterService || !this.realGardenerCluster) {
    return;
  }

  // Delete whatever this run created...
  for (const [, nodePoolId] of this.createdNodePoolIds) {
    try {
      await this.clusterService.deleteNodePool(nodePoolId);
    } catch (err) {
      console.error('cleanup: failed to delete tracked node pool', err);
    }
  }

  // ...and sweep any leftover "nomach" pool from a previous failed run that
  // never made it into createdNodePoolIds.
  try {
    const pools = await this.clusterService.listNodePools(
      this.realGardenerCluster.id,
    );
    const leftover = findNodePool(pools, NODE_POOL_NAME);
    if (leftover) {
      await this.clusterService.deleteNodePool(leftover.id);
    }
  } catch (err) {
    console.error('cleanup: failed to sweep leftover node pool', err);
  }
});

// --- Given Steps ---

Given(
  'a running cluster is available',
  { timeout: AUTHZ_READY_TIMEOUT_MS + 30_000 },
  async function (this: ICustomWorld) {
    this.authToken = await authenticateWithPassword(clusterOwnerEmail);
    this.organizationId = extractOrganizationId(this.authToken);
    this.currentUserEmail = clusterOwnerEmail;
    this.clusterService = new ClusterService(
      this.organizationApiUrl!,
      this.authToken,
      this.organizationId,
    );

    // Tolerate authz-worker -> OpenFGA propagation lag before this first
    // permission-gated call (see waitForAuthzReady above) - this is the only
    // call in the scenario that needs the retry, since every later call
    // happens well after the tuples have had time to land.
    const cluster = await waitForAuthzReady(() =>
      this.clusterService!.getClusterByName(clusterName),
    );
    expect(cluster, `cluster "${clusterName}" not found`).toBeDefined();
    this.realGardenerCluster = cluster;
  },
);

Given(
  'the seed has no free machine capacity',
  { timeout: 15_000 },
  async function (this: ICustomWorld) {
    cordonSeed(true);
    this.seedCordoned = true;
  },
);

// --- When Steps ---

When(
  'I add a node pool named {string} with autoscale {int} to {int}',
  { timeout: 30_000 },
  async function (
    this: ICustomWorld,
    name: string,
    autoscaleMin: number,
    autoscaleMax: number,
  ) {
    const response = await this.clusterService!.createNodePool({
      clusterId: this.realGardenerCluster!.id,
      name,
      machineType,
      autoscaleMin,
      autoscaleMax,
    });
    this.createdNodePoolIds.set(name, response.nodePoolId);
  },
);

When(
  'the seed capacity is restored',
  { timeout: 15_000 },
  async function (this: ICustomWorld) {
    cordonSeed(false);
    this.seedCordoned = false;
  },
);

// --- Then Steps ---

Then(
  'within {int} minutes the node pool {string} reports waiting for machines with a reason',
  { timeout: 12 * 60_000 },
  async function (this: ICustomWorld, minutes: number, name: string) {
    const pool = await pollUntil(
      minutes * 60_000,
      `node pool "${name}" reporting WAITING_FOR_MACHINES with a reason`,
      async () => {
        const pools = await this.clusterService!.listNodePools(
          this.realGardenerCluster!.id,
        );
        const found = findNodePool(pools, name);
        if (
          found &&
          found.status === NodePoolStatus.WAITING_FOR_MACHINES &&
          found.statusMessage !== ''
        ) {
          return found;
        }
        return undefined;
      },
    );
    expect(pool.status).toBe(NodePoolStatus.WAITING_FOR_MACHINES);
    expect(pool.statusMessage).not.toBe('');
  },
);

Then(
  'the cluster status is {string} or {string}',
  { timeout: 3 * 60_000 },
  async function (this: ICustomWorld, first: string, second: string) {
    const wanted = [clusterStatusByName(first), clusterStatusByName(second)];
    const cluster = await pollUntil(
      2 * 60_000,
      `cluster "${clusterName}" status in [${first}, ${second}]`,
      async () => {
        const details = await this.clusterService!.getClusterByName(
          clusterName,
        );
        return details && wanted.includes(details.status)
          ? details
          : undefined;
      },
    );
    expect(wanted).toContain(cluster.status);
  },
);

Then(
  'the activity log contains a {string} event',
  { timeout: 3 * 60_000 },
  async function (this: ICustomWorld, eventType: string) {
    const event = await pollUntil(
      2 * 60_000,
      `activity log containing a "${eventType}" event`,
      async () => {
        const events = await this.clusterService!.getClusterActivity(
          this.realGardenerCluster!.id,
        );
        return events.find((e) => e.eventType === eventType);
      },
    );
    expect(event.eventType).toBe(eventType);
  },
);

Then(
  'within {int} minutes the node pool {string} reports a healthy or provisioning state',
  { timeout: 17 * 60_000 },
  async function (this: ICustomWorld, minutes: number, name: string) {
    const pool = await pollUntil(
      minutes * 60_000,
      `node pool "${name}" reporting HEALTHY or PROVISIONING`,
      async () => {
        const pools = await this.clusterService!.listNodePools(
          this.realGardenerCluster!.id,
        );
        const found = findNodePool(pools, name);
        if (
          found &&
          (found.status === NodePoolStatus.HEALTHY ||
            found.status === NodePoolStatus.PROVISIONING)
        ) {
          return found;
        }
        return undefined;
      },
    );
    expect([NodePoolStatus.HEALTHY, NodePoolStatus.PROVISIONING]).toContain(
      pool.status,
    );
  },
);

Then(
  'the cluster status becomes {string}',
  { timeout: 6 * 60_000 },
  async function (this: ICustomWorld, status: string) {
    const wanted = clusterStatusByName(status);
    const cluster = await pollUntil(
      5 * 60_000,
      `cluster "${clusterName}" status becoming ${status}`,
      async () => {
        const details = await this.clusterService!.getClusterByName(
          clusterName,
        );
        return details && details.status === wanted ? details : undefined;
      },
    );
    expect(cluster.status).toBe(wanted);
  },
);
