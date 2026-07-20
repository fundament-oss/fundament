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
import {
  PluginSummarySchema,
  PluginDetailSchema,
  PresetSchema,
  TagSchema,
  CategorySchema,
  AuthorSchema,
  DocumentationLinkSchema,
  type PluginDetail,
} from '../../generated/v1/plugin_pb';
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

// --- Plugins --------------------------------------------------------------

// The catalog the walkthrough shows. Every `name` must have a matching icon at
// public/img/plugins/<name>.svg — the plugin card renders that path directly and
// has no fallback, so a plugin without an icon shows a broken image mid-demo.
// cert-manager and openfsc mirror the real definitions in plugins/*/definition.yaml.

const tag = (id: string, name: string) => create(TagSchema, { id, name });

const category = (id: string, name: string) => create(CategorySchema, { id, name });

const OFFICIAL = tag('tag-official', 'Official');

const CATEGORIES = {
  security: category('cat-security', 'Security'),
  networking: category('cat-networking', 'Networking'),
  observability: category('cat-observability', 'Observability'),
  data: category('cat-data', 'Data'),
  identity: category('cat-identity', 'Identity'),
};

const image = (name: string, version: string) =>
  `ghcr.io/fundament/plugins/${name}:${version}`;

// cert-manager is deliberately first: it is the card the platform-engineer tour
// auto-installs, and the drive script targets the first card in the grid.
export const plugins = [
  create(PluginSummarySchema, {
    id: 'pl-cert-manager',
    name: 'cert-manager',
    displayName: 'Cert Manager',
    descriptionShort: 'Automated TLS certificate management for Kubernetes.',
    description:
      'Automated TLS certificate management for Kubernetes using cert-manager. Vraagt certificaten aan, vernieuwt ze op tijd, en levert ze als secret aan je workloads.',
    tags: [OFFICIAL, tag('tag-tls', 'tls')],
    categories: [CATEGORIES.security],
    image: image('cert-manager', 'v1.17.2'),
  }),
  create(PluginSummarySchema, {
    id: 'pl-openfsc',
    name: 'openfsc',
    displayName: 'OpenFSC',
    descriptionShort: 'Federated Service Connectivity voor teams.',
    description:
      'Federated Service Connectivity (FSC) voor teams. Installeert de openfsc-operator; elk team declareert een FSCInstallation in zijn eigen namespace om daar een OpenFSC-peer te draaien.',
    tags: [OFFICIAL, tag('tag-fsc', 'fsc')],
    categories: [CATEGORIES.networking],
    image: image('openfsc', 'v4.0.0'),
  }),
  create(PluginSummarySchema, {
    id: 'pl-istio-gateway',
    name: 'istio-gateway',
    displayName: 'Istio Gateway',
    descriptionShort: 'Gateway API op basis van Istio: Gateways, HTTPRoutes en TLS.',
    description:
      'Gateway API-implementatie op basis van Istio. Beheert Gateways, HTTPRoutes, GRPCRoutes, TCPRoutes en TLSRoutes voor het verkeer je cluster in.',
    tags: [OFFICIAL, tag('tag-ingress', 'ingress')],
    categories: [CATEGORIES.networking],
    image: image('istio-gateway', 'v0.1.0'),
  }),
  create(PluginSummarySchema, {
    id: 'pl-sealed-secrets',
    name: 'sealed-secrets',
    displayName: 'Sealed Secrets',
    descriptionShort: 'Versleutelde secrets die je veilig in git kunt zetten.',
    description:
      'Versleutelt secrets zo dat alleen de controller in het cluster ze kan lezen. Daardoor kan de versleutelde versie gewoon mee in je repository.',
    tags: [tag('tag-secrets', 'secrets')],
    categories: [CATEGORIES.security],
    image: image('sealed-secrets', 'v0.27.1'),
  }),
  create(PluginSummarySchema, {
    id: 'pl-grafana',
    name: 'grafana',
    displayName: 'Grafana',
    descriptionShort: 'Dashboards en alerts voor je diensten.',
    description:
      'Grafana-dashboards voor je eigen diensten, met de metrics van het platform als basis. Alerts komen bij je eigen team terecht.',
    tags: [tag('tag-dashboards', 'dashboards')],
    categories: [CATEGORIES.observability],
    image: image('grafana', 'v11.4.0'),
  }),
  create(PluginSummarySchema, {
    id: 'pl-grafana-loki',
    name: 'grafana-loki',
    displayName: 'Grafana Loki',
    descriptionShort: 'Logs verzamelen en doorzoeken, per namespace.',
    description:
      'Verzamelt de logs van je workloads en maakt ze doorzoekbaar per namespace, zodat teams alleen hun eigen logs zien.',
    tags: [tag('tag-logs', 'logs')],
    categories: [CATEGORIES.observability],
    image: image('grafana-loki', 'v3.3.2'),
  }),
  create(PluginSummarySchema, {
    id: 'pl-cloudnativepg',
    name: 'cloudnativepg',
    displayName: 'CloudNativePG',
    descriptionShort: 'PostgreSQL als resource in je eigen namespace.',
    description:
      'Draait PostgreSQL-clusters in je eigen namespace, met back-ups en failover geregeld door de operator.',
    tags: [tag('tag-postgres', 'postgres')],
    categories: [CATEGORIES.data],
    image: image('cloudnativepg', 'v1.25.0'),
  }),
  create(PluginSummarySchema, {
    id: 'pl-keycloak',
    name: 'keycloak',
    displayName: 'Keycloak',
    descriptionShort: 'Inloggen en autorisatie voor je eigen dienst.',
    description:
      'Identity- en accessmanagement voor je eigen dienst: inloggen, rollen en tokens, zonder dat elk team het zelf bouwt.',
    tags: [tag('tag-sso', 'sso')],
    categories: [CATEGORIES.identity],
    image: image('keycloak', 'v26.0.7'),
  }),
];

export const presets = [
  create(PresetSchema, {
    id: 'preset-basis',
    name: 'Basisdiensten',
    description: 'Wat vrijwel elk cluster nodig heeft.',
    pluginIds: ['pl-cert-manager', 'pl-istio-gateway', 'pl-sealed-secrets'],
  }),
  create(PresetSchema, {
    id: 'preset-observability',
    name: 'Observability',
    description: 'Zien wat je dienst doet.',
    pluginIds: ['pl-grafana', 'pl-grafana-loki'],
  }),
  create(PresetSchema, {
    id: 'preset-data',
    name: 'Data & identiteit',
    description: 'Opslag en inloggen voor je eigen dienst.',
    pluginIds: ['pl-cloudnativepg', 'pl-keycloak'],
  }),
];

/** Detail view, derived from the summary so the two can never disagree. */
export const pluginDetail = (pluginId: string): PluginDetail | undefined => {
  const summary = plugins.find((p) => p.id === pluginId);
  if (!summary) return undefined;
  return create(PluginDetailSchema, {
    id: summary.id,
    name: summary.name,
    displayName: summary.displayName,
    description: summary.description,
    descriptionShort: summary.descriptionShort,
    tags: summary.tags,
    categories: summary.categories,
    author: create(AuthorSchema, { name: 'Fundament', url: 'https://fundament.dev' }),
    repositoryUrl: `https://github.com/fundament/plugins/tree/main/${summary.name}`,
    documentationLinks: [
      create(DocumentationLinkSchema, {
        id: `doc-${summary.name}`,
        title: 'Documentatie',
        urlName: 'docs',
        url: `https://docs.fundament.dev/plugins/${summary.name}`,
      }),
    ],
  });
};

/** Plugins already running when the walkthrough starts, per cluster. */
export const seededInstalls: Record<string, string[]> = {
  'cl-production': ['openfsc', 'grafana'],
  'cl-staging': ['openfsc'],
};

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
