/**
 * Steps for the @real-gardener node pool provisioning feature.
 *
 * This feature only passes against a real Gardener seed (not the mock used by
 * PR previews): it cordons/uncordons an actual kind node so the shoot's
 * machine-controller-manager genuinely cannot place machines, then watches
 * ClusterService report that through the API. The default cucumber profile
 * excludes @real-gardener (see cucumber.json); it runs in the test-env VM with
 * `just test-env test`.
 */

import { execFileSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { Given, When, Then, Before, After } from '@cucumber/cucumber';
import { expect } from '@playwright/test';
import { timestampDate } from '@bufbuild/protobuf/wkt';
import { ICustomWorld } from '../support/world.ts';
import { clusterName } from '../support/fixtures.ts';
import { shootForCluster, withShootAdmin } from '../support/shoot.ts';
import { ClusterService, type NodePool } from '../support/api/cluster-service.ts';
import { ClusterStatus, NodePoolStatus } from '../support/generated/v1/common_pb.ts';
import {
  authenticateWithPassword,
  extractOrganizationId,
  waitForAuthzReady,
  AUTHZ_READY_TIMEOUT_MS,
} from '../support/auth.ts';
import { pollUntil } from '../support/poll.ts';

// The scenario always targets this cluster/pool; the pool name is also
// hardcoded (not read from world state) so the After hook can sweep it even
// if a prior failed run never got far enough to record it.
const clusterOwnerEmail = process.env.E2E_CLUSTER_OWNER_EMAIL ?? 'alice@acme-corp.com';
const machineType = process.env.E2E_NODE_POOL_MACHINE_TYPE ?? 'local-small';
const NODE_POOL_NAME = 'nomach';

const seedContext = process.env.E2E_SEED_CONTEXT ?? 'kind-gardener-operator-local';
const seedNode = process.env.E2E_SEED_NODE ?? 'gardener-operator-local-control-plane';

// scenarioStartedAt bounds the activity-log assertions: on a reused cluster
// the log keeps nodepool_* events from earlier runs, and finding one of those
// must not pass the scenario.
let scenarioStartedAt = new Date(0);

// True while this run holds the cordon, so cleanup never undoes a cordon
// someone else set.
let seedCordoned = false;

function cordonSeed(on: boolean) {
  // Only a local seed: this cordons a whole node.
  if (!/^(kind|k3d)-/.test(seedContext)) {
    throw new Error(
      `E2E_SEED_CONTEXT "${seedContext}" is not a local kind/k3d context`,
    );
  }
  if (!on && !seedCordoned) {
    return;
  }
  execFileSync(
    'kubectl',
    ['--context', seedContext, on ? 'cordon' : 'uncordon', seedNode],
    { stdio: 'inherit' },
  );
  seedCordoned = on;
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
  scenarioStartedAt = new Date();
  this.organizationApiUrl =
    process.env.ORGANIZATION_API_URL ||
    'https://organization.fundament.localhost:8443';
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

  try {
    await sweepPool(this.clusterService, this.realGardenerCluster.id);
  } catch (err) {
    console.error('cleanup: failed to sweep leftover node pool', err);
  }
});

// The scenario only ever creates the one pool named NODE_POOL_NAME, so a
// sweep by name covers this run and any previous failed one alike. Says
// whether there was one.
async function sweepPool(service: ClusterService, clusterId: string): Promise<boolean> {
  const leftover = findNodePool(await service.listNodePools(clusterId), NODE_POOL_NAME);
  if (leftover) {
    await service.deleteNodePool(leftover.id);
  }
  return leftover !== undefined;
}

// A pool of the same name gets the same worker on the shoot, so a new one
// would take over the leftover's worker, and its machine once the seed has room.
const LEFTOVER_GONE_TIMEOUT_MS = 10 * 60_000;
async function waitLeftoverGone(world: ICustomWorld) {
  const clusterId = world.realGardenerCluster!.id;
  await pollUntil(LEFTOVER_GONE_TIMEOUT_MS, `the leftover node pool "${NODE_POOL_NAME}" to leave the shoot`, async () => {
    const shoot = shootForCluster(clusterId);
    const reconciled =
      shoot?.status?.observedGeneration === shoot?.metadata.generation &&
      shoot?.status?.lastOperation?.state === 'Succeeded';
    const hasWorker = shoot?.spec?.provider?.workers?.some((w) => w.name === workerName(NODE_POOL_NAME));
    if (!reconciled || hasWorker) {
      return undefined;
    }
    return (await nodesInPool(world, NODE_POOL_NAME)).length === 0 ? true : undefined;
  });
}

// --- Given Steps ---

Given(
  'a running cluster is available',
  { timeout: AUTHZ_READY_TIMEOUT_MS + LEFTOVER_GONE_TIMEOUT_MS + 60_000 },
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
      this.clusterService!.getClusterByName(clusterName()),
    );
    expect(cluster, `cluster "${clusterName()}" not found`).toBeDefined();
    this.realGardenerCluster = cluster;
    // A run killed before its After hook leaves the pool behind.
    if (await sweepPool(this.clusterService, cluster!.id)) {
      await waitLeftoverGone(this);
    }
  },
);

Given(
  'the seed has no free machine capacity',
  { timeout: 15_000 },
  async function (this: ICustomWorld) {
    cordonSeed(true);
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
    await this.clusterService!.createNodePool({
      clusterId: this.realGardenerCluster!.id,
      name,
      machineType,
      autoscaleMin,
      autoscaleMax,
    });
  },
);

When(
  'I delete the node pool {string}',
  { timeout: 30_000 },
  async function (this: ICustomWorld, name: string) {
    const pools = await this.clusterService!.listNodePools(this.realGardenerCluster!.id);
    const pool = findNodePool(pools, name);
    expect(pool, `node pool "${name}"`).toBeDefined();
    await this.clusterService!.deleteNodePool(pool!.id);
  },
);

When(
  'the seed capacity is restored',
  { timeout: 15_000 },
  async function (this: ICustomWorld) {
    cordonSeed(false);
  },
);

// --- Then Steps ---

Then(
  'within {int} minutes the node pool {string} reports waiting for machines with a reason',
  // No cucumber timeout: pollUntil enforces the {int} minutes the feature
  // names, so changing the feature cannot run into a hardcoded step timeout.
  { timeout: -1 },
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
      `cluster "${clusterName()}" status in [${first}, ${second}]`,
      async () => {
        const details = await this.clusterService!.getClusterByName(
          clusterName(),
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
      `activity log containing a "${eventType}" event from this run`,
      async () => {
        const events = await this.clusterService!.getClusterActivity(
          this.realGardenerCluster!.id,
        );
        // Only events from this run: a reused cluster keeps the nodepool_*
        // events of earlier runs in its log.
        return events.find(
          (e) =>
            e.eventType === eventType &&
            e.createdAt !== undefined &&
            timestampDate(e.createdAt) >= scenarioStartedAt,
        );
      },
    );
    expect(event.eventType).toBe(eventType);
  },
);

Then(
  'within {int} minutes the node pool {string} reports a healthy state',
  // No cucumber timeout: pollUntil enforces the {int} minutes the feature
  // names, so changing the feature cannot run into a hardcoded step timeout.
  { timeout: -1 },
  async function (this: ICustomWorld, minutes: number, name: string) {
    const pool = await pollUntil(
      minutes * 60_000,
      `node pool "${name}" reporting HEALTHY`,
      async () => {
        const pools = await this.clusterService!.listNodePools(
          this.realGardenerCluster!.id,
        );
        const found = findNodePool(pools, name);
        return found && found.status === NodePoolStatus.HEALTHY
          ? found
          : undefined;
      },
    );
    expect(pool.status).toBe(NodePoolStatus.HEALTHY);
  },
);

Then(
  'the cluster status becomes {string}',
  { timeout: 6 * 60_000 },
  async function (this: ICustomWorld, status: string) {
    const wanted = clusterStatusByName(status);
    const cluster = await pollUntil(
      5 * 60_000,
      `cluster "${clusterName()}" status becoming ${status}`,
      async () => {
        const details = await this.clusterService!.getClusterByName(
          clusterName(),
        );
        return details && details.status === wanted ? details : undefined;
      },
    );
    expect(cluster.status).toBe(wanted);
  },
);

// The shoot's worker for a pool: 4 characters of the name, padded with its
// hash, then 4 more of the hash (kubename.GenerateWorkerName).
function workerName(pool: string): string {
  let sanitized = pool.toLowerCase().replace(/[^a-z0-9]/g, '');
  if (/^[0-9]/.test(sanitized)) {
    sanitized = `x${sanitized}`;
  }
  const hash = createHash('sha256').update(pool).digest('hex');
  return (sanitized.slice(0, 4) + hash).slice(0, 4) + hash.slice(4, 8);
}

// Nodes of a pool, by Gardener's pool label.
async function nodesInPool(world: ICustomWorld, name: string): Promise<string[]> {
  return withShootAdmin(world.realGardenerCluster!.id, async (kubectl) => {
    const nodes = kubectl(['get', 'nodes', '-l', `worker.gardener.cloud/pool=${workerName(name)}`, '-o', 'name']);
    return nodes.split('\n').filter(Boolean);
  });
}

Then(
  'the cluster has {int} node(s) in the node pool {string}',
  { timeout: 6 * 60_000 },
  async function (this: ICustomWorld, count: number, name: string) {
    // A healthy pool's node may still be registering with the API server.
    const nodes = await pollUntil(5 * 60_000, `${count} node(s) in pool "${name}"`, async () => {
      const found = await nodesInPool(this, name);
      return found.length === count ? found : undefined;
    });
    expect(nodes).toHaveLength(count);
  },
);

Then(
  'within {int} minutes the cluster has no node in the node pool {string}',
  { timeout: -1 },
  async function (this: ICustomWorld, minutes: number, name: string) {
    await pollUntil(minutes * 60_000, `no node in pool "${name}"`, async () =>
      (await nodesInPool(this, name)).length === 0 ? true : undefined,
    );
  },
);

Then(
  'within {int} minutes the node pool {string} is no longer listed',
  { timeout: -1 },
  async function (this: ICustomWorld, minutes: number, name: string) {
    await pollUntil(minutes * 60_000, `node pool "${name}" to be gone`, async () =>
      findNodePool(await this.clusterService!.listNodePools(this.realGardenerCluster!.id), name) ? undefined : true,
    );
  },
);
