import { Injectable } from '@angular/core';

// Lifecycle status of an authored plugin as it moves through the
// "Pushed via functl -> Central review -> Publish" pipeline.
export type PluginStatus = 'pushed' | 'in_review' | 'changes_requested' | 'published';

export interface PluginAuthor {
  name: string;
  url: string;
}

export interface PluginVersion {
  version: string;
  pushedAt: string; // ISO date
  status: PluginStatus;
  // Optional reviewer note, e.g. why changes were requested.
  notes?: string;
}

export interface AuthoredPlugin {
  name: string; // stable identifier, used in URLs
  displayName: string;
  descriptionShort: string;
  description: string;
  version: string; // current/latest version
  author: PluginAuthor;
  repositoryUrl: string;
  image: string; // OCI image reference
  icon: string; // base name under /img/plugins/<icon>.svg
  tags: string[];
  category: string;
  installs: number; // number of clusters this plugin is currently installed on
  status: PluginStatus;
  versions: PluginVersion[];
}

// A cluster the author can sideload onto. Sideloading targets a normal cluster
// the user already owns; one of them is flagged as a development cluster.
export interface SideloadCluster {
  id: string;
  name: string;
  isDevelopment: boolean;
}

export interface SideloadRequest {
  image: string;
  version: string;
  displayName?: string;
  description?: string;
  clusterId: string;
}

// Hardcoded mock data. This service intentionally mimics the shape of the
// ConnectRPC-backed services (async methods returning promises) so it can be
// swapped for a real author-side API later without touching the components.
const MOCK_PLUGINS: AuthoredPlugin[] = [
  {
    name: 'postgres-operator',
    displayName: 'Postgres Operator',
    descriptionShort: 'Managed PostgreSQL clusters with automated backups and failover.',
    description:
      'Provision and operate production-grade PostgreSQL clusters on Kubernetes. Handles high availability, point-in-time recovery, connection pooling and automated minor-version upgrades.',
    version: '2.3.1',
    author: { name: 'Platform Data Team', url: 'https://example.gov/teams/data' },
    repositoryUrl: 'https://github.com/example-gov/postgres-operator',
    image: 'registry.fundament.io/plugins/postgres-operator:2.3.1',
    icon: 'cloudnativepg',
    tags: ['database', 'storage', 'official'],
    category: 'Database',
    installs: 38,
    status: 'published',
    versions: [
      { version: '2.3.1', pushedAt: '2026-06-18', status: 'published' },
      { version: '2.3.0', pushedAt: '2026-05-02', status: 'published' },
      { version: '2.2.4', pushedAt: '2026-03-11', status: 'published' },
    ],
  },
  {
    name: 'keycloak-sso',
    displayName: 'Keycloak SSO',
    descriptionShort: 'Single sign-on and identity brokering powered by Keycloak.',
    description:
      'Drop-in single sign-on for your workloads. Provides OIDC/SAML identity brokering, user federation and fine-grained authorization backed by a managed Keycloak instance.',
    version: '1.4.0',
    author: { name: 'Identity Guild', url: 'https://example.gov/teams/identity' },
    repositoryUrl: 'https://github.com/example-gov/keycloak-sso',
    image: 'registry.fundament.io/plugins/keycloak-sso:1.4.0',
    icon: 'keycloak',
    tags: ['security', 'identity'],
    category: 'Security',
    installs: 12,
    status: 'in_review',
    versions: [
      { version: '1.4.0', pushedAt: '2026-07-01', status: 'in_review' },
      { version: '1.3.2', pushedAt: '2026-04-22', status: 'published' },
    ],
  },
  {
    name: 'grafana-dashboards',
    displayName: 'Grafana Dashboards',
    descriptionShort: 'Curated observability dashboards for common platform workloads.',
    description:
      'A batteries-included set of Grafana dashboards and alert rules covering ingress, workloads and platform components. Installs alongside an existing Grafana instance.',
    version: '0.9.0',
    author: { name: 'Observability Crew', url: 'https://example.gov/teams/o11y' },
    repositoryUrl: 'https://github.com/example-gov/grafana-dashboards',
    image: 'registry.fundament.io/plugins/grafana-dashboards:0.9.0',
    icon: 'grafana',
    tags: ['observability', 'monitoring'],
    category: 'Observability',
    installs: 5,
    status: 'changes_requested',
    versions: [
      {
        version: '0.9.0',
        pushedAt: '2026-06-29',
        status: 'changes_requested',
        notes:
          'Requested RBAC scope is too broad. Please narrow the ClusterRole to the monitoring namespace and resubmit.',
      },
      { version: '0.8.1', pushedAt: '2026-05-15', status: 'published' },
    ],
  },
  {
    name: 'sealed-secrets',
    displayName: 'Sealed Secrets',
    descriptionShort: 'Encrypt secrets so they can safely live in Git.',
    description:
      'Encrypt Kubernetes Secrets into SealedSecrets that are safe to store in version control. The controller decrypts them in-cluster at apply time.',
    version: '0.1.0',
    author: { name: 'Internal Tooling', url: 'https://example.gov/teams/tooling' },
    repositoryUrl: 'https://github.com/example-gov/sealed-secrets',
    image: 'registry.fundament.io/plugins/sealed-secrets:0.1.0',
    icon: 'sealed-secrets',
    tags: ['security', 'internal'],
    category: 'Security',
    installs: 0,
    status: 'pushed',
    versions: [{ version: '0.1.0', pushedAt: '2026-07-06', status: 'pushed' }],
  },
];

const MOCK_CLUSTERS: SideloadCluster[] = [
  { id: 'cl-dev-01', name: 'team-sandbox', isDevelopment: true },
  { id: 'cl-prod-01', name: 'production-eu-west', isDevelopment: false },
  { id: 'cl-stg-01', name: 'staging-eu-west', isDevelopment: false },
];

@Injectable({ providedIn: 'root' })
export default class PluginDevelopmentService {
  private readonly plugins = MOCK_PLUGINS;

  private readonly clusters = MOCK_CLUSTERS;

  // Records sideload requests made during this session (mock only).
  private readonly sideloaded: SideloadRequest[] = [];

  listPlugins(): Promise<AuthoredPlugin[]> {
    return Promise.resolve(this.plugins.map((plugin) => ({ ...plugin })));
  }

  getPlugin(name: string): Promise<AuthoredPlugin | null> {
    const plugin = this.plugins.find((p) => p.name === name);
    return Promise.resolve(plugin ? { ...plugin } : null);
  }

  listClusters(): Promise<SideloadCluster[]> {
    return Promise.resolve(this.clusters.map((cluster) => ({ ...cluster })));
  }

  // Mock sideload: records the request and resolves successfully.
  sideload(request: SideloadRequest): Promise<void> {
    this.sideloaded.push(request);
    return Promise.resolve();
  }
}
