// Handwritten catalog for the demo build. Mirrors the plugins in
// console-frontend/src/app/demo/fixtures.ts — ids, names and display names are
// deliberately identical, so a viewer moving from a marketplace slide to a
// console slide sees the same catalogue rather than two unrelated ones.
//
// Every `name` must have a matching icon at public/img/plugins/<name>.svg: the
// plugin card renders that path directly and has no fallback.
import { create } from '@bufbuild/protobuf';
import { timestampFromDate } from '@bufbuild/protobuf/wkt';
import {
  PluginSummarySchema,
  PluginDetailsSchema,
  PublishedVersionSchema,
  type PluginSummary,
  type PluginDetails,
  type PublishedVersion,
} from '../../generated/catalog/v1/catalog_pb';
import { PluginLabel } from '../../generated/catalog/v1/common_pb';
import {
  CategorySchema,
  PublisherSchema,
  PluginPermissionSchema,
  FeatureBlockSchema,
  DocumentationLinkSchema,
  SubmissionStatus,
} from '../../generated/marketplace/v1/common_pb';
import {
  PluginSchema as RegistryPluginSchema,
  PluginVersionSchema as RegistryPluginVersionSchema,
  type Plugin as RegistryPlugin,
  type PluginVersion as RegistryPluginVersion,
} from '../../generated/registry/v1/common_pb';

const daysAgo = (n: number) => timestampFromDate(new Date(Date.now() - n * 86_400_000));

// --- Publishers -----------------------------------------------------------

export const SYSTEM_ORG_ID = 'org-fundament';
const VNG_ORG_ID = 'org-vng';

export const publishers = [
  create(PublisherSchema, {
    id: SYSTEM_ORG_ID,
    name: 'fundament',
    displayName: 'Fundament',
  }),
  create(PublisherSchema, {
    id: VNG_ORG_ID,
    name: 'vng',
    displayName: 'VNG Realisatie',
  }),
];

// --- Categories -----------------------------------------------------------

const CATEGORY_IDS = {
  security: 'cat-security',
  networking: 'cat-networking',
  observability: 'cat-observability',
  data: 'cat-data',
  identity: 'cat-identity',
};

export const categories = [
  create(CategorySchema, { id: CATEGORY_IDS.security, name: 'Security' }),
  create(CategorySchema, { id: CATEGORY_IDS.networking, name: 'Networking' }),
  create(CategorySchema, { id: CATEGORY_IDS.observability, name: 'Observability' }),
  create(CategorySchema, { id: CATEGORY_IDS.data, name: 'Data' }),
  create(CategorySchema, { id: CATEGORY_IDS.identity, name: 'Identity' }),
];

// --- Versions -------------------------------------------------------------

// Latest first, matching the console fixtures. The first entry is the version a
// listing advertises; a plugin without one reads as "not published yet".
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

const versionId = (name: string, version: string) => `ver-${name}-${version}`;

// Shaped like a real sha256 and deterministic, so the same version always
// resolves to the same hash.
function definitionHash(name: string, version: string): string {
  const seed = `${name}@${version}`;
  const hash = [...seed].reduce((acc, char) => (acc * 31 + char.charCodeAt(0)) % 0xffffffff, 7);
  return `sha256:${hash.toString(16).padStart(8, '0').repeat(8)}`;
}

/** The catalog fields that follow from a plugin's latest published version. */
const published = (name: string, ageDays: number) => ({
  organizationId: SYSTEM_ORG_ID,
  latestVersionId: versionId(name, VERSIONS[name]?.[0] ?? ''),
  published: daysAgo(ageDays),
});

// --- Listings -------------------------------------------------------------

// cert-manager is deliberately first: the storefront drive clicks the first
// card, and the console tour installs cert-manager on the slide after it.
export const plugins: PluginSummary[] = [
  create(PluginSummarySchema, {
    id: 'pl-cert-manager',
    name: 'cert-manager',
    displayName: 'Cert Manager',
    descriptionShort: 'Automated TLS certificate management for Kubernetes.',
    categoryIds: [CATEGORY_IDS.security],
    tags: ['tls', 'certificates'],
    labels: [PluginLabel.CORE, PluginLabel.SUPPORT_9_TO_17],
    ...published('cert-manager', 6),
  }),
  create(PluginSummarySchema, {
    id: 'pl-openfsc',
    name: 'openfsc',
    displayName: 'OpenFSC',
    descriptionShort: 'Federated Service Connectivity voor teams.',
    categoryIds: [CATEGORY_IDS.networking],
    tags: ['fsc', 'connectivity'],
    labels: [PluginLabel.CORE, PluginLabel.RIJKSOVERHEID],
    ...published('openfsc', 12),
  }),
  create(PluginSummarySchema, {
    id: 'pl-istio-gateway',
    name: 'istio-gateway',
    displayName: 'Istio Gateway',
    descriptionShort: 'Gateway API op basis van Istio: Gateways, HTTPRoutes en TLS.',
    categoryIds: [CATEGORY_IDS.networking],
    tags: ['ingress', 'gateway-api'],
    labels: [PluginLabel.CORE],
    ...published('istio-gateway', 30),
  }),
  create(PluginSummarySchema, {
    id: 'pl-sealed-secrets',
    name: 'sealed-secrets',
    displayName: 'Sealed Secrets',
    descriptionShort: 'Versleutelde secrets die je veilig in git kunt zetten.',
    categoryIds: [CATEGORY_IDS.security],
    tags: ['secrets', 'gitops'],
    labels: [PluginLabel.SUPPORT_9_TO_17],
    ...published('sealed-secrets', 45),
  }),
  create(PluginSummarySchema, {
    id: 'pl-grafana',
    name: 'grafana',
    displayName: 'Grafana',
    descriptionShort: 'Dashboards en alerts voor je diensten.',
    categoryIds: [CATEGORY_IDS.observability],
    tags: ['dashboards', 'metrics'],
    labels: [PluginLabel.SUPPORT_9_TO_17],
    ...published('grafana', 60),
  }),
  create(PluginSummarySchema, {
    id: 'pl-grafana-loki',
    name: 'grafana-loki',
    displayName: 'Grafana Loki',
    descriptionShort: 'Logs verzamelen en doorzoeken, per namespace.',
    categoryIds: [CATEGORY_IDS.observability],
    tags: ['logs'],
    labels: [],
    ...published('grafana-loki', 75),
  }),
  create(PluginSummarySchema, {
    id: 'pl-cloudnativepg',
    name: 'cloudnativepg',
    displayName: 'CloudNativePG',
    descriptionShort: 'PostgreSQL als resource in je eigen namespace.',
    categoryIds: [CATEGORY_IDS.data],
    tags: ['postgres', 'database'],
    labels: [PluginLabel.SUPPORT_9_TO_17],
    ...published('cloudnativepg', 90),
  }),
  create(PluginSummarySchema, {
    id: 'pl-keycloak',
    name: 'keycloak',
    displayName: 'Keycloak',
    descriptionShort: 'Inloggen en autorisatie voor je eigen dienst.',
    categoryIds: [CATEGORY_IDS.identity],
    tags: ['sso', 'oidc'],
    labels: [PluginLabel.RIJKSOVERHEID],
    ...published('keycloak', 120),
  }),
];

const nameById = (pluginId: string) => plugins.find((plugin) => plugin.id === pluginId)?.name ?? '';

/** Published version history for a plugin, latest first. */
export function pluginVersions(pluginId: string): PublishedVersion[] {
  const name = nameById(pluginId);
  return (VERSIONS[name] ?? []).map((version, index) =>
    create(PublishedVersionSchema, {
      id: versionId(name, version),
      version,
      published: daysAgo(14 + index * 45),
      definitionHash: definitionHash(name, version),
      releaseNotes: index === 0 ? 'Bijgewerkte upstream chart en dichtgezette CVEs.' : '',
    }),
  );
}

// --- Listing detail -------------------------------------------------------

// Only the fields the detail page adds on top of the summary. cert-manager is
// filled in fully because it is the listing every tour opens; the rest carry
// enough to render without holes.
const DETAILS: Record<
  string,
  {
    description: string;
    capabilities?: string[];
    permissions?: { resource: string; access: string }[];
    features?: { title: string; body: string }[];
    documentationLinks?: { title: string; urlName: string; url: string }[];
  }
> = {
  'cert-manager': {
    description:
      'Automated TLS certificate management for Kubernetes. Vraagt certificaten aan, vernieuwt ze op tijd, en levert ze als secret aan je workloads. Je team beheert zijn certificaten als gewone resources in de console, zonder kubectl en zonder apart dashboard.',
    capabilities: ['internet_access', 'cluster_wide'],
    permissions: [
      { resource: 'Certificates', access: 'Lezen en schrijven' },
      { resource: 'Secrets', access: 'Aanmaken en bijwerken' },
      { resource: 'Ingresses', access: 'Alleen lezen' },
    ],
    features: [
      {
        title: 'Automatisch vernieuwen',
        body: 'Certificaten worden ruim voor de vervaldatum vernieuwd, zonder dat iemand een herinnering in zijn agenda hoeft te zetten.',
      },
      {
        title: 'Eigen schermen in de console',
        body: 'De plugin brengt zijn eigen menu mee. Elk project op het cluster krijgt Certificates in de zijbalk.',
      },
      {
        title: 'Werkt met elke uitgever',
        body: "Let's Encrypt, een interne CA of PKIoverheid: de issuer is een resource als elke andere.",
      },
    ],
    documentationLinks: [
      { title: 'Documentatie', urlName: 'Aan de slag', url: 'https://cert-manager.io/docs/' },
      { title: 'Broncode', urlName: 'GitHub', url: 'https://github.com/cert-manager/cert-manager' },
    ],
  },
  openfsc: {
    description:
      'Federated Service Connectivity (FSC) voor teams. Installeert de openfsc-operator; elk team declareert een FSCInstallation in zijn eigen namespace om daar een OpenFSC-peer te draaien.',
    capabilities: ['internet_access'],
    permissions: [
      { resource: 'FSCInstallations', access: 'Lezen en schrijven' },
      { resource: 'Services', access: 'Alleen lezen' },
    ],
  },
  'istio-gateway': {
    description:
      'Gateway API-implementatie op basis van Istio. Beheert Gateways, HTTPRoutes, GRPCRoutes, TCPRoutes en TLSRoutes voor het verkeer je cluster in.',
    permissions: [{ resource: 'Gateways en routes', access: 'Lezen en schrijven' }],
  },
  'sealed-secrets': {
    description:
      'Versleutelt secrets zo dat alleen de controller in het cluster ze kan lezen. Daardoor kan de versleutelde versie gewoon mee in je repository.',
    permissions: [{ resource: 'Secrets', access: 'Aanmaken en bijwerken' }],
  },
  grafana: {
    description:
      'Grafana-dashboards voor je eigen diensten, met de metrics van het platform als basis. Alerts komen bij je eigen team terecht.',
  },
  'grafana-loki': {
    description:
      'Verzamelt de logs van je workloads en maakt ze doorzoekbaar per namespace, zodat teams alleen hun eigen logs zien.',
  },
  cloudnativepg: {
    description:
      'Draait PostgreSQL-clusters in je eigen namespace, met back-ups en failover geregeld door de operator.',
  },
  keycloak: {
    description:
      'Identity- en accessmanagement voor je eigen dienst: inloggen, rollen en tokens, zonder dat elk team het zelf bouwt.',
  },
};

/** The detail listing for a plugin, or undefined for an unknown id. */
export function pluginDetails(pluginId: string): PluginDetails | undefined {
  const summary = plugins.find((plugin) => plugin.id === pluginId);
  if (!summary) return undefined;
  const extra = DETAILS[summary.name] ?? { description: summary.descriptionShort };
  return create(PluginDetailsSchema, {
    id: summary.id,
    name: summary.name,
    displayName: summary.displayName,
    descriptionShort: summary.descriptionShort,
    organizationId: summary.organizationId,
    image: summary.image,
    categoryIds: summary.categoryIds,
    tags: summary.tags,
    labels: summary.labels,
    latestVersionId: summary.latestVersionId,
    published: summary.published,
    description: extra.description,
    authorName: 'Fundament',
    authorUrl: 'https://fundament.projects.digilab.network/',
    repositoryUrl: `https://github.com/fundament-oss/fundament/tree/master/plugins/${summary.name}`,
    license: 'EUPL-1.2',
    capabilities: extra.capabilities ?? [],
    permissions: (extra.permissions ?? []).map((permission) =>
      create(PluginPermissionSchema, permission),
    ),
    features: (extra.features ?? []).map((feature) => create(FeatureBlockSchema, feature)),
    documentationLinks: (extra.documentationLinks ?? []).map((link, index) =>
      create(DocumentationLinkSchema, { id: `doc-${summary.name}-${index}`, ...link }),
    ),
  });
}

// --- Developer surface (registry.v1) --------------------------------------

// What "My plugins" shows: the listings this organization published, plus one
// build still working its way through review. The statuses are picked so the
// page shows every state the status column can render.
const AUTHORED: {
  id: string;
  name: string;
  displayName: string;
  descriptionShort: string;
  description: string;
  categoryId: string;
  tags: string[];
  versions: { version: string; status: SubmissionStatus; ageDays: number; feedback?: string }[];
}[] = [
  {
    id: 'pl-cert-manager',
    name: 'cert-manager',
    displayName: 'Cert Manager',
    descriptionShort: 'Automated TLS certificate management for Kubernetes.',
    description: DETAILS['cert-manager'].description,
    categoryId: CATEGORY_IDS.security,
    tags: ['tls', 'certificates'],
    versions: [
      { version: 'v1.17.3', status: SubmissionStatus.PENDING, ageDays: 2 },
      { version: 'v1.17.2', status: SubmissionStatus.APPROVED, ageDays: 14 },
      { version: 'v1.17.1', status: SubmissionStatus.APPROVED, ageDays: 59 },
    ],
  },
  {
    id: 'pl-openfsc',
    name: 'openfsc',
    displayName: 'OpenFSC',
    descriptionShort: 'Federated Service Connectivity voor teams.',
    description: DETAILS['openfsc'].description,
    categoryId: CATEGORY_IDS.networking,
    tags: ['fsc', 'connectivity'],
    versions: [{ version: 'v4.0.0', status: SubmissionStatus.APPROVED, ageDays: 12 }],
  },
  {
    id: 'pl-istio-gateway',
    name: 'istio-gateway',
    displayName: 'Istio Gateway',
    descriptionShort: 'Gateway API op basis van Istio: Gateways, HTTPRoutes en TLS.',
    description: DETAILS['istio-gateway'].description,
    categoryId: CATEGORY_IDS.networking,
    tags: ['ingress', 'gateway-api'],
    versions: [
      {
        version: 'v0.2.0',
        status: SubmissionStatus.CHANGES_REQUESTED,
        ageDays: 5,
        feedback:
          'Graag de permissies in de beschrijving toelichten: de listing vraagt cluster-brede rechten zonder uitleg.',
      },
      { version: 'v0.1.0', status: SubmissionStatus.APPROVED, ageDays: 30 },
    ],
  },
  {
    id: 'pl-vng-zaakregistratie',
    name: 'keycloak',
    displayName: 'Zaakregistratie',
    descriptionShort: 'Zaakgericht werken als resource in je eigen namespace.',
    description:
      'Een zaakregistratiecomponent voor gemeenten, geleverd als plugin. Nog niet ingediend: dit is de draft die na de eerste functl push is aangemaakt.',
    categoryId: CATEGORY_IDS.data,
    tags: ['zaakgericht', 'common ground'],
    versions: [{ version: 'v0.1.0', status: SubmissionStatus.DRAFT, ageDays: 1 }],
  },
];

export const authoredPlugins: RegistryPlugin[] = AUTHORED.map((plugin) =>
  create(RegistryPluginSchema, {
    id: plugin.id,
    name: plugin.name,
    displayName: plugin.displayName,
    descriptionShort: plugin.descriptionShort,
    description: plugin.description,
    organizationId: SYSTEM_ORG_ID,
    categoryIds: [plugin.categoryId],
    tags: plugin.tags,
    authorName: 'Fundament',
    authorUrl: 'https://fundament.projects.digilab.network/',
    repositoryUrl: `https://github.com/fundament-oss/fundament/tree/master/plugins/${plugin.name}`,
    license: 'EUPL-1.2',
    created: daysAgo(180),
    updated: daysAgo(2),
  }),
);

// Mutable: submitting or withdrawing a version on the developer surface has to
// stick for the rest of the slide, the way an install does in the console demo.
export const authoredVersions = new Map<string, RegistryPluginVersion[]>(
  AUTHORED.map((plugin) => [
    plugin.id,
    plugin.versions.map((version) =>
      create(RegistryPluginVersionSchema, {
        id: versionId(plugin.name, version.version),
        pluginId: plugin.id,
        version: version.version,
        image: `ghcr.io/fundament-oss/fundament/plugins/${plugin.name}:${version.version}`,
        definitionHash: definitionHash(plugin.name, version.version),
        status: version.status,
        created: daysAgo(version.ageDays),
        submitted:
          version.status === SubmissionStatus.DRAFT ? undefined : daysAgo(version.ageDays - 1),
        published:
          version.status === SubmissionStatus.APPROVED ? daysAgo(version.ageDays - 1) : undefined,
        reviewFeedback: version.feedback ?? '',
      }),
    ),
  ]),
);

/** The version rows for one authored plugin, or [] for an unknown id. */
export function versionsForPlugin(pluginId: string): RegistryPluginVersion[] {
  return authoredVersions.get(pluginId) ?? [];
}

/** Looks a version up across every authored plugin, for submit and withdraw. */
export function findVersion(versionId_: string): RegistryPluginVersion | undefined {
  return [...authoredVersions.values()].flat().find((version) => version.id === versionId_);
}
