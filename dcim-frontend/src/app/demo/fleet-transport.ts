// Demo-only in-memory fleet.v1 (FUN-23): a handful of organizations and
// clusters, so the Platform section can be walked through without
// platform-api. Never imported by the production entrypoint (main.ts).
import { create } from '@bufbuild/protobuf';
import { timestampFromDate } from '@bufbuild/protobuf/wkt';
import { Code, ConnectError, Transport, createRouterTransport } from '@connectrpc/connect';
import {
  ClusterFilter,
  ClusterHealth,
  ClusterStatus,
  DriftKind,
  FleetService,
  ClusterSummarySchema,
  DriftFindingSchema,
  OrganizationSummarySchema,
  type ClusterSummary,
  type OrganizationSummary,
} from '../../generated/fleet/v1/fleet_pb';

const STALE_AFTER_SECONDS = 45 * 60;

const minutesAgo = (n: number) => timestampFromDate(new Date(Date.now() - n * 60_000));
const daysAgo = (n: number) => minutesAgo(n * 24 * 60);

const ORGANIZATIONS = [
  { id: 'org-utrecht', name: 'gemeente-utrecht', alias: 'Gemeente Utrecht', maxNodes: 20 },
  { id: 'org-zwolle', name: 'gemeente-zwolle', alias: 'Gemeente Zwolle', maxNodes: 6 },
  { id: 'org-rvo', name: 'rvo', alias: 'Rijksdienst voor Ondernemend Nederland' },
];

interface DemoCluster {
  id: string;
  organizationId: string;
  name: string;
  status: ClusterStatus;
  health: ClusterHealth;
  pools: { name: string; min: number; max: number; nodes: number }[];
  observedMinutesAgo?: number;
  lastError?: string;
  namespaces: { project: string; name: string; pods: number }[];
}

const CLUSTERS: DemoCluster[] = [
  {
    id: 'cl-utrecht-prod',
    organizationId: 'org-utrecht',
    name: 'productie',
    status: ClusterStatus.RUNNING,
    health: ClusterHealth.HEALTHY,
    pools: [
      { name: 'workers', min: 3, max: 9, nodes: 5 },
      { name: 'batch', min: 0, max: 4, nodes: 0 },
    ],
    observedMinutesAgo: 6,
    namespaces: [
      { project: 'parkeren', name: 'parkeren-prod', pods: 14 },
      { project: 'parkeren', name: 'parkeren-acc', pods: 6 },
      { project: 'afval', name: 'afval-prod', pods: 9 },
    ],
  },
  {
    id: 'cl-utrecht-test',
    organizationId: 'org-utrecht',
    name: 'test',
    status: ClusterStatus.UPDATING,
    health: ClusterHealth.HEALTHY,
    pools: [{ name: 'workers', min: 1, max: 3, nodes: 2 }],
    observedMinutesAgo: 9,
    namespaces: [{ project: 'parkeren', name: 'parkeren-test', pods: 4 }],
  },
  {
    id: 'cl-zwolle-prod',
    organizationId: 'org-zwolle',
    name: 'productie',
    status: ClusterStatus.RUNNING,
    health: ClusterHealth.UNHEALTHY,
    pools: [{ name: 'workers', min: 2, max: 6, nodes: 6 }],
    observedMinutesAgo: 70,
    lastError: 'list nodes: Get "https://api.zwolle-prod": dial tcp: i/o timeout',
    namespaces: [{ project: 'vergunningen', name: 'vergunningen', pods: 11 }],
  },
  {
    id: 'cl-rvo-prod',
    organizationId: 'org-rvo',
    name: 'subsidies',
    status: ClusterStatus.RUNNING,
    health: ClusterHealth.HEALTHY,
    pools: [{ name: 'workers', min: 2, max: 8, nodes: 3 }],
    observedMinutesAgo: 3,
    namespaces: [{ project: 'subsidies', name: 'subsidies-prod', pods: 22 }],
  },
];

const PODS_PER_NODE = 110;

const nodesOf = (c: DemoCluster) => c.pools.reduce((sum, p) => sum + p.nodes, 0);
const podsOf = (c: DemoCluster) =>
  c.namespaces.reduce((sum, ns) => sum + ns.pods, 0) + nodesOf(c) * 8;

function organizationOf(id: string) {
  return ORGANIZATIONS.find((o) => o.id === id)!;
}

function clusterSummary(c: DemoCluster): ClusterSummary {
  const organization = organizationOf(c.organizationId);
  const nodes = nodesOf(c);
  const observed =
    c.observedMinutesAgo === undefined ? undefined : minutesAgo(c.observedMinutesAgo);
  return create(ClusterSummarySchema, {
    id: c.id,
    organizationId: c.organizationId,
    organizationName: organization.name,
    name: c.name,
    region: 'nl-ams-1',
    kubernetesVersion: '1.33.4',
    status: c.status,
    health: c.health,
    created: daysAgo(120),
    nodePools: c.pools.length,
    nodesMin: c.pools.reduce((sum, p) => sum + p.min, 0),
    nodesMax: c.pools.reduce((sum, p) => sum + p.max, 0),
    snapshot: {
      observed,
      nodes,
      nodesReady: c.health === ClusterHealth.UNHEALTHY ? nodes - 1 : nodes,
      pods: podsOf(c),
      podAllocatable: nodes * PODS_PER_NODE,
      cpuAllocatableM: BigInt(nodes * 3920),
      memoryAllocatableMi: BigInt(nodes * 15 * 1024),
      lastError: c.lastError,
      lastErrorAt: c.lastError ? minutesAgo(2) : undefined,
    },
    stale: (c.observedMinutesAgo ?? Infinity) * 60 > STALE_AFTER_SECONDS,
    overLimit: organization.maxNodes !== undefined && nodes >= organization.maxNodes,
  });
}

function organizationSummary(o: (typeof ORGANIZATIONS)[number]): OrganizationSummary {
  const clusters = CLUSTERS.filter((c) => c.organizationId === o.id);
  const summaries = clusters.map(clusterSummary);
  return create(OrganizationSummarySchema, {
    id: o.id,
    name: o.name,
    alias: o.alias,
    created: daysAgo(400),
    limits: { maxNodesPerCluster: o.maxNodes },
    clusters: clusters.length,
    clustersUnhealthy: clusters.filter((c) => c.health === ClusterHealth.UNHEALTHY).length,
    nodes: summaries.reduce((sum, c) => sum + (c.snapshot?.nodes ?? 0), 0),
    nodesReady: summaries.reduce((sum, c) => sum + (c.snapshot?.nodesReady ?? 0), 0),
    pods: summaries.reduce((sum, c) => sum + (c.snapshot?.pods ?? 0), 0),
    namespaces: clusters.reduce((sum, c) => sum + c.namespaces.length, 0),
  });
}

const findings = () => [
  create(DriftFindingSchema, {
    kind: DriftKind.SNAPSHOT_STALE,
    organizationId: 'org-zwolle',
    organizationName: 'gemeente-zwolle',
    clusterId: 'cl-zwolle-prod',
    clusterName: 'productie',
    since: minutesAgo(70),
    detail: 'Last collected 70 min ago. Last error: dial tcp: i/o timeout',
  }),
  create(DriftFindingSchema, {
    kind: DriftKind.SHOOT_OF_DELETED_CLUSTER,
    organizationId: 'org-rvo',
    organizationName: 'rvo',
    clusterId: 'cl-rvo-old',
    clusterName: 'archief',
    shootName: 'archief-7f3c',
    shootNamespace: 'garden-rvo',
    since: minutesAgo(4),
    detail:
      'The cluster is deleted but Gardener still lists its shoot. Reconcile deletes orphaned shoots, so this should clear by itself.',
  }),
];

const delay = (ms = 120) =>
  new Promise((resolve) => {
    setTimeout(resolve, ms);
  });

export default function createFleetDemoTransport(): Transport {
  return createRouterTransport((router) => {
    router.service(FleetService, {
      getFleetSummary: async () => {
        await delay();
        const clusters = CLUSTERS.map(clusterSummary);
        const sum = (pick: (c: ClusterSummary) => number) =>
          clusters.reduce((total, c) => total + pick(c), 0);
        return {
          summary: {
            organizations: ORGANIZATIONS.length,
            clusters: clusters.length,
            clustersUnhealthy: clusters.filter((c) => c.health === ClusterHealth.UNHEALTHY).length,
            clustersUpdating: clusters.filter((c) => c.status === ClusterStatus.UPDATING).length,
            nodes: sum((c) => c.snapshot?.nodes ?? 0),
            nodesReady: sum((c) => c.snapshot?.nodesReady ?? 0),
            pods: sum((c) => c.snapshot?.pods ?? 0),
            podAllocatable: sum((c) => c.snapshot?.podAllocatable ?? 0),
            cpuAllocatableM: BigInt(sum((c) => Number(c.snapshot?.cpuAllocatableM ?? 0n))),
            memoryAllocatableMi: BigInt(sum((c) => Number(c.snapshot?.memoryAllocatableMi ?? 0n))),
            driftFindings: findings().length,
            staleSnapshots: findings().filter((f) => f.kind === DriftKind.SNAPSHOT_STALE).length,
            gardenerInventoryRefreshed: minutesAgo(2),
            staleAfterSeconds: STALE_AFTER_SECONDS,
          },
        };
      },
      listOrganizations: async () => {
        await delay();
        return { organizations: ORGANIZATIONS.map(organizationSummary) };
      },
      getOrganization: async (req) => {
        await delay();
        const organization = ORGANIZATIONS.find((o) => o.id === req.organizationId);
        if (!organization) throw new ConnectError('organization not found', Code.NotFound);
        return { organization: organizationSummary(organization) };
      },
      listClusters: async (req) => {
        await delay();
        const clusters = CLUSTERS.filter(
          (c) => !req.organizationId || c.organizationId === req.organizationId,
        ).map(clusterSummary);
        switch (req.filter) {
          case ClusterFilter.UNHEALTHY:
            return { clusters: clusters.filter((c) => c.health === ClusterHealth.UNHEALTHY) };
          case ClusterFilter.STALE:
            return { clusters: clusters.filter((c) => c.stale) };
          case ClusterFilter.OVER_LIMIT:
            return { clusters: clusters.filter((c) => c.overLimit) };
          case ClusterFilter.ALL:
          case ClusterFilter.UNSPECIFIED:
            return { clusters };
          default:
            throw new Error(`unhandled cluster filter ${req.filter satisfies never}`);
        }
      },
      getCluster: async (req) => {
        await delay();
        const cluster = CLUSTERS.find((c) => c.id === req.clusterId);
        if (!cluster) throw new ConnectError('cluster not found', Code.NotFound);
        const organization = organizationOf(cluster.organizationId);
        const projects = [...new Set(cluster.namespaces.map((ns) => ns.project))];
        return {
          cluster: clusterSummary(cluster),
          limits: { maxNodesPerCluster: organization.maxNodes, maxNodesPerNodePool: 6 },
          shoot: {
            name: `${cluster.name}-${cluster.id.slice(-4)}`,
            namespace: `garden-${organization.name.slice(0, 6)}`,
            kubernetesVersion: '1.33.4',
            region: 'nl-ams-1',
            created: daysAgo(120),
            lastSeen: minutesAgo(2),
          },
          nodePools: cluster.pools.map((p, i) => ({
            id: `${cluster.id}-pool-${i}`,
            name: p.name,
            machineType: 'standard-4',
            autoscaleMin: p.min,
            autoscaleMax: p.max,
            nodes: p.nodes,
            nodesReady: p.nodes,
            observed: minutesAgo(cluster.observedMinutesAgo ?? 0),
          })),
          projects: projects.map((name, i) => ({
            id: `${cluster.id}-project-${i}`,
            name,
            alias: name,
            namespaces: cluster.namespaces
              .filter((ns) => ns.project === name)
              .map((ns, j) => ({
                id: `${cluster.id}-ns-${i}-${j}`,
                name: ns.name,
                pods: ns.pods,
                observed: minutesAgo(cluster.observedMinutesAgo ?? 0),
              })),
          })),
        };
      },
      listDriftFindings: async () => {
        await delay();
        return { findings: findings() };
      },
    });
  });
}
