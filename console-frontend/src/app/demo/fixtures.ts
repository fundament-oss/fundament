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
  RegionSchema,
  RegionMachineTypeSchema,
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
  PluginDefinitionVersionSchema,
  type PluginDetail,
  type PluginDefinitionVersion,
} from '../../generated/v1/plugin_pb';
import { ClusterStatus, NodePoolStatus } from '../../generated/v1/common_pb';
import { UserSchema } from '../../generated/authn/v1/authn_pb';
import type { KubeResource, ParsedCrd, PluginDefinition } from '../plugin-resources/types';

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
  // The name is the machine-readable one the address goes by; the alias is the
  // label people read.
  name: 'gemeente-fundament',
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

// Mutable so the new-cluster form's createCluster can append a new one.
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

export const clusterDetails = new Map(
  [
    create(ClusterDetailsSchema, {
      id: 'cl-production',
      name: 'production',
      region: 'local',
      kubernetesVersion: '1.34.0',
      status: ClusterStatus.RUNNING,
      created: daysAgo(180),
    }),
    create(ClusterDetailsSchema, {
      id: 'cl-staging',
      name: 'staging',
      region: 'local',
      kubernetesVersion: '1.33.0',
      status: ClusterStatus.RUNNING,
      created: daysAgo(90),
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
]);

// --- Region catalog -------------------------------------------------------

// Backs ClusterService.ListRegions, which the new-cluster form loads before it
// renders (it shows an error instead of the form when the call fails) and which the
// node pool pages use to fill the machine type dropdown.
//
// `local` must stay first: the form defaults to the first region, and it is the
// region the cluster fixtures above already use. Every machineType referenced by
// nodePoolsByCluster must appear here, otherwise the node pool forms offer a list
// that cannot reproduce the pools shown next to them.

const gib = (n: number) => BigInt(n) * 1073741824n;

const machineType = (name: string, lcpu: number, memoryGib: number) =>
  create(RegionMachineTypeSchema, { name, lcpu, memory: gib(memoryGib) });

const MACHINE_TYPES = [
  machineType('e2-standard-2', 2, 8),
  machineType('e2-standard-4', 4, 16),
  machineType('e2-highmem-4', 4, 32),
];

export const regions = [
  create(RegionSchema, {
    name: 'local',
    kubernetesVersions: ['1.34.0', '1.33.0', '1.32.5'],
    machineTypes: MACHINE_TYPES,
  }),
  create(RegionSchema, {
    name: 'nl-north-1',
    kubernetesVersions: ['1.34.0', '1.33.0'],
    machineTypes: MACHINE_TYPES,
  }),
];

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
  // The count on a project comes from its own memberCount, so a project with a
  // count and no members shows "2 members" over an empty list.
  [
    'pr-belastingen',
    [
      create(ProjectMemberSchema, {
        id: 'pm-4',
        projectId: 'pr-belastingen',
        userId: 'user-demo',
        userName: 'Demi de Demonstratie',
        role: ProjectMemberRole.ADMIN,
        created: daysAgo(140),
      }),
      create(ProjectMemberSchema, {
        id: 'pm-5',
        projectId: 'pr-belastingen',
        userId: 'user-omar',
        userName: 'Omar El Amrani',
        role: ProjectMemberRole.VIEWER,
        created: daysAgo(20),
      }),
    ],
  ],
  [
    'pr-burgerzaken-staging',
    [
      create(ProjectMemberSchema, {
        id: 'pm-6',
        projectId: 'pr-burgerzaken-staging',
        userId: 'user-demo',
        userName: 'Demi de Demonstratie',
        role: ProjectMemberRole.ADMIN,
        created: daysAgo(60),
      }),
      create(ProjectMemberSchema, {
        id: 'pm-7',
        projectId: 'pr-burgerzaken-staging',
        userId: 'user-sanne',
        userName: 'Sanne Bakker',
        role: ProjectMemberRole.VIEWER,
        created: daysAgo(45),
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

// The platform's starting values, hardcoded in organization-api's
// limit_defaults.go. Handing back the saved limits instead would make every
// project look like it is still on the platform's numbers.
export const platformProjectLimits = create(ProjectLimitsSchema, {
  defaultMemoryRequestMi: 256,
  defaultMemoryLimitMi: 512,
  defaultCpuRequestM: 100,
  defaultCpuLimitM: 500,
});

export const platformOrganizationLimits = create(OrganizationLimitsSchema, {
  maxNodesPerCluster: 10,
  maxNodePoolsPerCluster: 5,
  maxNodesPerNodePool: 5,
  defaultMemoryRequestMi: 256,
  defaultMemoryLimitMi: 512,
  defaultCpuRequestM: 100,
  defaultCpuLimitM: 500,
});

// --- Metrics --------------------------------------------------------------

// A demo needs charts with something in them, so the series are generated: a
// smooth base with a daily rhythm and a little noise, deterministic per metric
// so a reload shows the same picture.
const sample = (index: number, base: number, swing: number, seed: number): number => {
  const wave = Math.sin((index / 12 + seed) * Math.PI * 2) * swing;
  const jitter = Math.sin(index * (1.7 + seed)) * swing * 0.25;
  return Math.max(0, Number((base + wave + jitter).toFixed(2)));
};

/** `count` samples ending now, `stepSeconds` apart. */
export const metricSeries = (
  count: number,
  stepSeconds: number,
  base: number,
  swing: number,
  seed: number,
  now: number,
) =>
  Array.from({ length: count }, (_, index) => ({
    timestamp: new Date(now - (count - 1 - index) * stepSeconds * 1000),
    value: sample(index, base, swing, seed),
  }));

export const namespaceMetrics = [
  {
    namespace: 'burgerzaken-prod',
    cpuCores: 1.42,
    memoryGib: 6.1,
    pods: 14,
    cpuRequests: 2,
    cpuLimits: 4,
    memoryRequestsGib: 8,
    memoryLimitsGib: 16,
    networkReceiveMbS: 1.8,
    networkTransmitMbS: 0.9,
  },
  {
    namespace: 'belastingen-prod',
    cpuCores: 0.72,
    memoryGib: 3.4,
    pods: 8,
    cpuRequests: 1,
    cpuLimits: 2,
    memoryRequestsGib: 4,
    memoryLimitsGib: 8,
    networkReceiveMbS: 0.6,
    networkTransmitMbS: 0.4,
  },
  {
    namespace: 'burgerzaken-staging',
    cpuCores: 0.26,
    memoryGib: 1.2,
    pods: 6,
    cpuRequests: 0.5,
    cpuLimits: 1,
    memoryRequestsGib: 2,
    memoryLimitsGib: 4,
    networkReceiveMbS: 0.2,
    networkTransmitMbS: 0.1,
  },
];

export const clusterUsage = [
  {
    clusterId: 'cl-production',
    clusterName: 'production',
    cpu: { used: 2.4, total: 8, unit: 'cores' },
    memory: { used: 12.8, total: 32, unit: 'GiB' },
    pods: { used: 28, total: 110, unit: 'pods' },
  },
  {
    clusterId: 'cl-staging',
    clusterName: 'staging',
    cpu: { used: 0.9, total: 4, unit: 'cores' },
    memory: { used: 3.6, total: 16, unit: 'GiB' },
    pods: { used: 9, total: 55, unit: 'pods' },
  },
];

export const nodeUsage = [
  {
    node: 'general-0',
    cpu: { used: 1.1, total: 4, unit: 'cores' },
    memory: { used: 6.2, total: 16, unit: 'GiB' },
    pods: { used: 14, total: 55, unit: 'pods' },
  },
  {
    node: 'general-1',
    cpu: { used: 0.8, total: 4, unit: 'cores' },
    memory: { used: 4.4, total: 16, unit: 'GiB' },
    pods: { used: 9, total: 55, unit: 'pods' },
  },
];

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

// The published definitions per plugin, latest first — what the install modal's
// version picker offers and what an install pins. The first entry is the version
// the catalog card advertises; without one a plugin reads as "not published yet"
// and cannot be installed at all.
const VERSIONS: Record<string, string[]> = {
  'cert-manager': ['v1.17.2', 'v1.17.1', 'v1.16.3'],
  openfsc: ['v4.0.0', 'v3.2.1'],
  'istio-gateway': ['v0.1.0'],
  'sealed-secrets': ['v0.27.1', 'v0.26.3'],
  grafana: ['v11.4.0', 'v11.3.1'],
  'grafana-loki': ['v3.3.2', 'v3.2.0'],
  cloudnativepg: ['v1.25.0', 'v1.24.2'],
  keycloak: ['v26.0.7', 'v26.0.5'],
};

const latestVersion = (name: string) => VERSIONS[name]?.[0] ?? '';

// Stands in for the sha256 of a published manifest: shaped like the real thing and
// deterministic, so the same version always resolves to the same hash — the install
// pins it and the plugin's installation shows it back.
const definitionHash = (name: string, version: string) => {
  const digest = `${name}@${version}`
    .split('')
    .reduce((acc, char) => (acc * 31 + char.charCodeAt(0)) % 0xffffffff, 7);
  return `sha256:${digest.toString(16).padStart(8, '0').repeat(8)}`;
};

/** The catalog fields that follow from a plugin's latest published version. */
const published = (name: string) => ({
  // First-party demo plugins are published by the seeded 'system' organization,
  // mirroring db/migrations/033_org-owned-plugins.up.sql.
  organizationName: 'system',
  image: `ghcr.io/fundament/plugins/${name}:${latestVersion(name)}`,
  pluginVersion: latestVersion(name),
  definitionHash: definitionHash(name, latestVersion(name)),
});

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
    ...published('cert-manager'),
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
    ...published('openfsc'),
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
    ...published('istio-gateway'),
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
    ...published('sealed-secrets'),
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
    ...published('grafana'),
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
    ...published('grafana-loki'),
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
    ...published('cloudnativepg'),
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
    ...published('keycloak'),
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
    organizationName: summary.organizationName,
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
    pluginVersion: summary.pluginVersion,
    definitionHash: summary.definitionHash,
  });
};

/**
 * Published definitions of a plugin, latest first — what the install modal's version
 * picker offers. An unknown plugin gets an empty list, the same "nothing published"
 * answer the real backend gives.
 */
export const pluginDefinitionVersions = (pluginId: string): PluginDefinitionVersion[] => {
  const summary = plugins.find((p) => p.id === pluginId);
  if (!summary) return [];
  return (VERSIONS[summary.name] ?? []).map((version) =>
    create(PluginDefinitionVersionSchema, {
      version,
      hash: definitionHash(summary.name, version),
    }),
  );
};

/** Plugins already running when the walkthrough starts, per cluster. */
export const seededInstalls: Record<string, string[]> = {
  'cl-production': ['openfsc', 'grafana'],
  'cl-staging': ['openfsc'],
};

// --- Plugin UI ------------------------------------------------------------
//
// What the console normally reads from the cluster through kube-api-proxy: the
// plugin definition behind an installation, its CRDs, and the objects of those
// CRDs. Only cert-manager carries a project menu here — that is the plugin the
// walkthrough installs, and a plugin without a menu simply has no console UI,
// exactly as in production.
//
// No customComponents on purpose: a custom UI is an iframe served by
// plugin-proxy, which the static demo has no backend for. Without it the
// console renders its own UI from the CRD schema below.

const certificateCrd: ParsedCrd = {
  group: 'cert-manager.io',
  kind: 'Certificate',
  plural: 'certificates',
  singular: 'certificate',
  scope: 'Namespaced',
  version: 'v1',
  additionalPrinterColumns: [
    { name: 'Ready', type: 'string', jsonPath: '.status.conditions[?(@.type=="Ready")].status' },
    { name: 'Secret', type: 'string', jsonPath: '.spec.secretName' },
    { name: 'Issuer', type: 'string', jsonPath: '.spec.issuerRef.name' },
    { name: 'Age', type: 'date', jsonPath: '.metadata.creationTimestamp' },
  ],
  specSchema: {
    required: ['secretName', 'issuerRef'],
    properties: {
      commonName: { type: 'string', description: 'Common name of the requested certificate.' },
      dnsNames: {
        type: 'array',
        description: 'DNS names the certificate is valid for.',
        items: { type: 'string' },
      },
      secretName: {
        type: 'string',
        description: 'Name of the secret the issued certificate is written to.',
      },
      duration: { type: 'string', description: 'How long the certificate stays valid.' },
      renewBefore: { type: 'string', description: 'How long before expiry to renew.' },
      issuerRef: {
        type: 'object',
        description: 'The issuer that signs this certificate.',
        required: ['name', 'kind'],
        properties: {
          name: { type: 'string' },
          kind: { type: 'string', enum: ['Issuer', 'ClusterIssuer'] },
        },
      },
    },
  },
  statusSchema: {
    properties: {
      notAfter: { type: 'string', format: 'date-time', description: 'Expiry of the certificate.' },
      renewalTime: { type: 'string', format: 'date-time', description: 'Next renewal attempt.' },
    },
  },
};

/** Parsed CRDs per plugin, in place of the ones kube-api-proxy would serve. */
export const pluginCrds: Record<string, ParsedCrd[]> = {
  'cert-manager': [certificateCrd],
};

/** Plugin definitions per catalog name; an installation without one has no UI. */
export const pluginDefinitions: Record<string, PluginDefinition> = {
  'cert-manager': {
    name: 'cert-manager',
    label: 'Cert Manager',
    version: 'v1.17.2',
    description: 'Certificaten die het platform zelf aanvraagt en op tijd vernieuwt.',
    author: 'Fundament',
    // Menu entries reference a CRD by `plural.group`, exactly as the real
    // definition.yaml in plugins/cert-manager does.
    menu: { project: [{ crd: 'certificates.cert-manager.io', icon: 'certificate' }] },
    crds: ['certificates.cert-manager.io'],
    allowedResources: [
      { group: 'cert-manager.io', version: 'v1', resource: 'certificates', verbs: ['get', 'list'] },
    ],
    installationId: 'demo-cert-manager',
    installationName: 'system--cert-manager',
    installationVersion: 'v1.17.2',
    organizationName: 'system',
  },
};

const certificate = (
  name: string,
  namespace: string,
  dnsName: string,
  ready: boolean,
  ageDays: number,
): KubeResource => ({
  apiVersion: 'cert-manager.io/v1',
  kind: 'Certificate',
  metadata: {
    name,
    namespace,
    uid: `demo-cert-${name}`,
    creationTimestamp: new Date(Date.now() - ageDays * 86_400_000).toISOString(),
  },
  spec: {
    commonName: dnsName,
    dnsNames: [dnsName],
    secretName: `${name}-tls`,
    duration: '2160h0m0s',
    renewBefore: '720h0m0s',
    issuerRef: { name: 'letsencrypt', kind: 'ClusterIssuer' },
  },
  status: {
    conditions: [
      {
        type: 'Ready',
        status: ready ? 'True' : 'False',
        reason: ready ? 'Ready' : 'InProgress',
        message: ready
          ? 'Certificate is up to date and has not expired'
          : 'Waiting for the issuer to sign the request',
        // A ready certificate last changed state when it was issued; one still
        // being signed changed state just now. The detail view shows this column.
        lastTransitionTime: new Date(Date.now() - (ready ? ageDays * 86_400_000 : 0)).toISOString(),
      },
    ],
    notAfter: new Date(Date.now() + 60 * 86_400_000).toISOString(),
    renewalTime: new Date(Date.now() + 30 * 86_400_000).toISOString(),
  },
});

/** Objects per `${pluginName}/${kind}`, in place of a live cluster's. */
export const pluginResources: Record<string, KubeResource[]> = {
  'cert-manager/Certificate': [
    certificate('burgerzaken-portaal', 'burgerzaken-prod', 'burgerzaken.gemeente.nl', true, 90),
    certificate('burgerzaken-api', 'burgerzaken-prod', 'api.burgerzaken.gemeente.nl', true, 42),
    certificate('burgerzaken-afspraken', 'burgerzaken-prod', 'afspraken.gemeente.nl', false, 0),
  ],
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
