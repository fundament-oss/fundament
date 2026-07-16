// Demo-only, handwritten fixtures for the static walkthrough build.
// Not referenced by the production entrypoint (src/main.ts), so it is tree-shaken
// out of the production bundle.
import { create } from '@bufbuild/protobuf';
import { timestampFromDate } from '@bufbuild/protobuf/wkt';
import { OrganizationSchema, OrganizationLimitsSchema } from '../../generated/v1/organization_pb';
import {
  ListClustersResponse_ClusterSummarySchema,
  ClusterDetailsSchema,
  NodePoolSchema,
  ClusterEventSchema,
  ResourceUsageInfoSchema,
  type NodePool,
} from '../../generated/v1/cluster_pb';
import { NamespaceSchema } from '../../generated/v1/namespace_pb';
import {
  ProjectSchema,
  ProjectMemberSchema,
  ProjectLimitsSchema,
  ProjectMemberRole,
} from '../../generated/v1/project_pb';
import { MemberSchema } from '../../generated/v1/member_pb';
import { ClusterStatus, NodePoolStatus, ResourceUsageSchema } from '../../generated/v1/common_pb';
import { UserSchema } from '../../generated/authn/v1/authn_pb';

const daysAgo = (n: number) => timestampFromDate(new Date(Date.now() - n * 86_400_000));

export const ORG_ID = 'org-fundament';

export const demoUser = create(UserSchema, {
  id: 'user-demo',
  name: 'Demi de Demonstratie',
  organizationIds: [ORG_ID],
  groups: ['platform-team'],
});

export const organization = create(OrganizationSchema, {
  id: ORG_ID,
  name: 'Gemeente Fundament',
  alias: 'fundament',
  created: daysAgo(420),
});

export const organizationLimits = create(OrganizationLimitsSchema, {
  maxNodesPerCluster: 20,
  maxNodePoolsPerCluster: 5,
  maxNodesPerNodePool: 10,
  defaultMemoryRequestMi: 256,
  defaultMemoryLimitMi: 512,
  defaultCpuRequestM: 250,
  defaultCpuLimitM: 500,
});

// --- Clusters -------------------------------------------------------------

// Mutable so the add-cluster wizard's createCluster can append a new one.
export const clusterSummaries = [
  create(ListClustersResponse_ClusterSummarySchema, {
    id: 'cl-production',
    name: 'production',
    status: ClusterStatus.RUNNING,
    region: 'local',
    projectCount: 2,
    nodePoolCount: 2,
  }),
  create(ListClustersResponse_ClusterSummarySchema, {
    id: 'cl-staging',
    name: 'staging',
    status: ClusterStatus.RUNNING,
    region: 'local',
    projectCount: 1,
    nodePoolCount: 1,
  }),
];

const usage = (used: number, total: number, unit: string) =>
  create(ResourceUsageSchema, { used, total, unit });

export const clusterDetails = new Map(
  [
    create(ClusterDetailsSchema, {
      id: 'cl-production',
      name: 'production',
      region: 'local',
      kubernetesVersion: '1.34.0',
      status: ClusterStatus.RUNNING,
      created: daysAgo(180),
      observabilityUrl: 'https://grafana.fundament.example/d/production',
      resourceUsage: create(ResourceUsageInfoSchema, {
        cpu: usage(5200, 16000, 'm'),
        memory: usage(11, 32, 'Gi'),
        pods: usage(48, 330, 'pods'),
        disk: usage(120, 500, 'Gi'),
      }),
    }),
    create(ClusterDetailsSchema, {
      id: 'cl-staging',
      name: 'staging',
      region: 'local',
      kubernetesVersion: '1.33.0',
      status: ClusterStatus.RUNNING,
      created: daysAgo(90),
      observabilityUrl: 'https://grafana.fundament.example/d/staging',
      resourceUsage: create(ResourceUsageInfoSchema, {
        cpu: usage(1800, 8000, 'm'),
        memory: usage(4, 16, 'Gi'),
        pods: usage(17, 220, 'pods'),
        disk: usage(40, 250, 'Gi'),
      }),
    }),
  ].map((c) => [c.id, c] as const),
);

export const nodePoolsByCluster = new Map<string, NodePool[]>([
    [
      'cl-production',
      [
        create(NodePoolSchema, {
          id: 'np-general',
          name: 'general',
          machineType: 'e2-standard-4',
          currentNodes: 3,
          minNodes: 2,
          maxNodes: 6,
          status: NodePoolStatus.HEALTHY,
          version: '1.34.0',
        }),
        create(NodePoolSchema, {
          id: 'np-memory',
          name: 'memory-optimized',
          machineType: 'e2-highmem-4',
          currentNodes: 1,
          minNodes: 1,
          maxNodes: 3,
          status: NodePoolStatus.HEALTHY,
          version: '1.34.0',
        }),
      ],
    ],
    [
      'cl-staging',
      [
        create(NodePoolSchema, {
          id: 'np-general',
          name: 'general',
          machineType: 'e2-standard-2',
          currentNodes: 2,
          minNodes: 1,
          maxNodes: 4,
          status: NodePoolStatus.HEALTHY,
          version: '1.33.0',
        }),
      ],
    ],
  ],
);

export const clusterActivity = [
  create(ClusterEventSchema, {
    id: 'ev-1',
    eventType: 'NodePoolScaled',
    createdAt: daysAgo(0),
    syncAction: 'reconcile',
    message: 'Node pool "general" scaled from 2 to 3 nodes.',
    attempt: 1,
  }),
  create(ClusterEventSchema, {
    id: 'ev-2',
    eventType: 'ClusterReady',
    createdAt: daysAgo(1),
    syncAction: 'create',
    message: 'Cluster reconciliation completed successfully.',
    attempt: 1,
  }),
];

// --- Namespaces -----------------------------------------------------------

export const namespaces = [
  create(NamespaceSchema, {
    id: 'ns-burgerzaken-prod',
    name: 'burgerzaken-prod',
    projectId: 'pr-burgerzaken',
    clusterId: 'cl-production',
    created: daysAgo(120),
  }),
  create(NamespaceSchema, {
    id: 'ns-belastingen-prod',
    name: 'belastingen-prod',
    projectId: 'pr-belastingen',
    clusterId: 'cl-production',
    created: daysAgo(95),
  }),
  create(NamespaceSchema, {
    id: 'ns-burgerzaken-staging',
    name: 'burgerzaken-staging',
    projectId: 'pr-burgerzaken-staging',
    clusterId: 'cl-staging',
    created: daysAgo(60),
  }),
];

// --- Projects -------------------------------------------------------------

export const projects = [
  create(ProjectSchema, {
    id: 'pr-burgerzaken',
    clusterId: 'cl-production',
    name: 'burgerzaken',
    alias: 'burgerzaken',
    created: daysAgo(160),
    namespaceCount: 1,
    memberCount: 3,
  }),
  create(ProjectSchema, {
    id: 'pr-belastingen',
    clusterId: 'cl-production',
    name: 'belastingen',
    alias: 'belastingen',
    created: daysAgo(140),
    namespaceCount: 1,
    memberCount: 2,
  }),
  create(ProjectSchema, {
    id: 'pr-burgerzaken-staging',
    clusterId: 'cl-staging',
    name: 'burgerzaken',
    alias: 'burgerzaken-staging',
    created: daysAgo(60),
    namespaceCount: 1,
    memberCount: 2,
  }),
];

export const projectMembersByProject = new Map([
  [
    'pr-burgerzaken',
    [
      create(ProjectMemberSchema, {
        id: 'pm-1',
        projectId: 'pr-burgerzaken',
        userId: 'user-demo',
        userName: 'Demi de Demonstratie',
        role: ProjectMemberRole.ADMIN,
        created: daysAgo(160),
      }),
      create(ProjectMemberSchema, {
        id: 'pm-2',
        projectId: 'pr-burgerzaken',
        userId: 'user-sanne',
        userName: 'Sanne Bakker',
        role: ProjectMemberRole.ADMIN,
        created: daysAgo(120),
      }),
      create(ProjectMemberSchema, {
        id: 'pm-3',
        projectId: 'pr-burgerzaken',
        userId: 'user-omar',
        userName: 'Omar El Amrani',
        role: ProjectMemberRole.VIEWER,
        created: daysAgo(30),
      }),
    ],
  ],
]);

export const projectLimits = create(ProjectLimitsSchema, {
  defaultMemoryRequestMi: 256,
  defaultMemoryLimitMi: 512,
  defaultCpuRequestM: 250,
  defaultCpuLimitM: 500,
});

// --- Organization members -------------------------------------------------

export const members = [
  create(MemberSchema, {
    id: 'mb-1',
    userId: 'user-demo',
    name: 'Demi de Demonstratie',
    externalRef: 'demi',
    email: 'demi@fundament.example',
    permission: 'admin',
    status: 'active',
    created: daysAgo(420),
  }),
  create(MemberSchema, {
    id: 'mb-2',
    userId: 'user-sanne',
    name: 'Sanne Bakker',
    externalRef: 'sanne',
    email: 'sanne@fundament.example',
    permission: 'member',
    status: 'active',
    created: daysAgo(300),
  }),
  create(MemberSchema, {
    id: 'mb-3',
    userId: 'user-omar',
    name: 'Omar El Amrani',
    externalRef: 'omar',
    email: 'omar@fundament.example',
    permission: 'member',
    status: 'active',
    created: daysAgo(30),
  }),
];
