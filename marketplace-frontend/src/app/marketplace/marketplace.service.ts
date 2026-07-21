import { Injectable } from '@angular/core';

// Public marketplace catalog. This dataset is independent from the author-side
// "My plugins" mock (plugin-development.service.ts): it represents the plugins
// as consumers browse them, so it only contains published listings from a range
// of vendors.
//
// The shape is modelled loosely on the `appstore` schema described in FUN-11
// (plugins, tags, categories, documentation links) so it can later be swapped
// for the real PluginService API without reworking the components.

export interface DocumentationLink {
  label: string;
  url: string;
}

export interface PluginPermission {
  // Human-readable resource group, e.g. "Certificates" or "Networking".
  resource: string;
  // Short description of what the plugin does with it.
  access: string;
}

export interface FeatureBlock {
  title: string;
  body: string;
}

export interface MarketplacePlugin {
  name: string; // stable slug, used in URLs
  displayName: string;
  tagline: string; // one-line summary shown on cards
  description: string; // longer paragraph shown on the detail page
  vendor: string;
  icon: string; // base name under /img/plugins/<icon>.svg
  category: string;
  tags: string[];
  official: boolean;
  version: string;
  addedAt: string; // ISO date, used to sort "recently added"
  featured: boolean;
  // Declared capabilities (e.g. internet access) the plugin needs.
  capabilities: string[];
  // RBAC-style permissions, shown on the detail page.
  permissions: PluginPermission[];
  features: FeatureBlock[];
  documentationLinks: DocumentationLink[];
}

export interface Category {
  id: string; // matches MarketplacePlugin.category
  name: string;
}

const PLUGINS: MarketplacePlugin[] = [
  {
    name: 'cert-manager',
    displayName: 'Cert Manager',
    tagline: 'Automated TLS certificate management for your clusters.',
    description:
      'Cert Manager automatically provisions and renews TLS certificates from a range of issuers, including Let’s Encrypt and internal CAs. It watches Certificate resources and keeps secrets up to date so your workloads always serve valid certificates.',
    vendor: 'Fundament',
    icon: 'cert-manager',
    category: 'Security',
    tags: ['certificates', 'tls', 'security'],
    official: true,
    version: 'v1.17.2',
    addedAt: '2026-02-10',
    featured: true,
    capabilities: ['internet_access'],
    permissions: [
      { resource: 'Certificates', access: 'Read and write' },
      { resource: 'Issuers & ClusterIssuers', access: 'Read and write' },
      { resource: 'Secrets', access: 'Read and write' },
    ],
    features: [
      {
        title: 'Issue certificates automatically',
        body: 'Request a certificate with a single Kubernetes resource and Cert Manager handles the ACME challenge, issuance and storage for you.',
      },
      {
        title: 'Renew before expiry',
        body: 'Certificates are renewed well ahead of their expiry date, so there are no surprise outages from expired TLS.',
      },
    ],
    documentationLinks: [
      { label: 'Documentation', url: 'https://cert-manager.io/docs' },
      { label: 'Homepage', url: 'https://cert-manager.io' },
    ],
  },
  {
    name: 'istio',
    displayName: 'Istio Service Mesh',
    tagline: 'Traffic management, security and observability for your services.',
    description:
      'Istio adds a service mesh to your cluster, giving you mTLS between workloads, fine-grained traffic routing, retries and circuit breaking, plus rich telemetry — all without changing application code.',
    vendor: 'Fundament',
    icon: 'istio',
    category: 'Networking',
    tags: ['service-mesh', 'networking', 'security'],
    official: true,
    version: 'v1.24.0',
    addedAt: '2026-03-04',
    featured: true,
    capabilities: [],
    permissions: [
      { resource: 'VirtualServices & Gateways', access: 'Read and write' },
      { resource: 'DestinationRules', access: 'Read and write' },
      { resource: 'Pods', access: 'Read-only' },
    ],
    features: [
      {
        title: 'Zero-trust networking',
        body: 'Mutual TLS is enabled between all meshed workloads by default, so traffic inside the cluster is encrypted and authenticated.',
      },
      {
        title: 'Progressive delivery',
        body: 'Shift traffic between versions with weighted routing to run canary and blue/green releases safely.',
      },
    ],
    documentationLinks: [{ label: 'Documentation', url: 'https://istio.io/latest/docs' }],
  },
  {
    name: 'istio-gateway',
    displayName: 'Istio Gateway',
    tagline: 'Managed ingress gateway built on the Istio mesh.',
    description:
      'A ready-to-use ingress gateway for clusters running the Istio service mesh. Terminates TLS at the edge and routes external traffic to meshed workloads using Gateway and VirtualService resources.',
    vendor: 'Fundament',
    icon: 'istio-gateway',
    category: 'Networking',
    tags: ['ingress', 'networking', 'gateway'],
    official: true,
    version: 'v1.24.0',
    addedAt: '2026-03-04',
    featured: false,
    capabilities: [],
    permissions: [
      { resource: 'Gateways', access: 'Read and write' },
      { resource: 'Services', access: 'Read and write' },
    ],
    features: [
      {
        title: 'Edge TLS termination',
        body: 'Terminate HTTPS at the gateway and forward traffic over mTLS to your services.',
      },
    ],
    documentationLinks: [
      {
        label: 'Documentation',
        url: 'https://istio.io/latest/docs/tasks/traffic-management/ingress',
      },
    ],
  },
  {
    name: 'grafana',
    displayName: 'Grafana',
    tagline: 'Dashboards and visualisation for all your metrics and logs.',
    description:
      'Grafana gives your teams a single place to explore metrics, logs and traces. Ships with sensible default dashboards for platform components and lets you build your own on top of the observability stack.',
    vendor: 'Grafana Labs',
    icon: 'grafana',
    category: 'Observability',
    tags: ['observability', 'dashboards', 'monitoring'],
    official: false,
    version: 'v11.3.0',
    addedAt: '2026-04-18',
    featured: true,
    capabilities: ['internet_access'],
    permissions: [
      { resource: 'ConfigMaps', access: 'Read and write' },
      { resource: 'Services', access: 'Read-only' },
    ],
    features: [
      {
        title: 'Batteries-included dashboards',
        body: 'Preconfigured dashboards for ingress, workloads and platform components appear as soon as the plugin is installed.',
      },
    ],
    documentationLinks: [{ label: 'Documentation', url: 'https://grafana.com/docs/grafana' }],
  },
  {
    name: 'grafana-loki',
    displayName: 'Grafana Loki',
    tagline: 'Horizontally scalable log aggregation, like Prometheus for logs.',
    description:
      'Loki collects and indexes logs by label rather than full text, making it cost-efficient to run at scale. Query your logs from Grafana alongside your metrics.',
    vendor: 'Grafana Labs',
    icon: 'grafana-loki',
    category: 'Observability',
    tags: ['logs', 'observability'],
    official: false,
    version: 'v3.2.0',
    addedAt: '2026-05-22',
    featured: false,
    capabilities: [],
    permissions: [{ resource: 'PersistentVolumeClaims', access: 'Read and write' }],
    features: [
      {
        title: 'Label-based indexing',
        body: 'Only metadata is indexed, keeping storage costs low while still enabling fast, targeted log queries.',
      },
    ],
    documentationLinks: [{ label: 'Documentation', url: 'https://grafana.com/docs/loki' }],
  },
  {
    name: 'grafana-tempo',
    displayName: 'Grafana Tempo',
    tagline: 'High-scale distributed tracing backend.',
    description:
      'Tempo is a cost-effective distributed tracing backend that only requires object storage. Correlate traces with your metrics and logs directly in Grafana.',
    vendor: 'Grafana Labs',
    icon: 'grafana-tempo',
    category: 'Observability',
    tags: ['tracing', 'observability'],
    official: false,
    version: 'v2.6.0',
    addedAt: '2026-06-11',
    featured: false,
    capabilities: [],
    permissions: [{ resource: 'PersistentVolumeClaims', access: 'Read and write' }],
    features: [
      {
        title: 'Traces to logs',
        body: 'Jump from a trace span straight to the relevant logs in Loki to debug faster.',
      },
    ],
    documentationLinks: [{ label: 'Documentation', url: 'https://grafana.com/docs/tempo' }],
  },
  {
    name: 'grafana-mimir',
    displayName: 'Grafana Mimir',
    tagline: 'Long-term, highly available storage for Prometheus metrics.',
    description:
      'Mimir provides durable, long-term storage for Prometheus metrics with horizontal scalability and a highly available query path. Keep years of metrics without running out of local disk.',
    vendor: 'Grafana Labs',
    icon: 'grafana-mimir',
    category: 'Observability',
    tags: ['metrics', 'observability'],
    official: false,
    version: 'v2.14.0',
    addedAt: '2026-06-25',
    featured: false,
    capabilities: [],
    permissions: [{ resource: 'PersistentVolumeClaims', access: 'Read and write' }],
    features: [
      {
        title: 'Unlimited retention',
        body: 'Store metrics in object storage for as long as you need, decoupled from your Prometheus instances.',
      },
    ],
    documentationLinks: [{ label: 'Documentation', url: 'https://grafana.com/docs/mimir' }],
  },
  {
    name: 'grafana-alloy',
    displayName: 'Grafana Alloy',
    tagline: 'A flexible collector for metrics, logs, traces and profiles.',
    description:
      'Alloy is an OpenTelemetry collector distribution that gathers telemetry from your workloads and ships it to Loki, Mimir and Tempo. One agent for your whole observability pipeline.',
    vendor: 'Grafana Labs',
    icon: 'grafana-alloy',
    category: 'Observability',
    tags: ['agent', 'observability', 'opentelemetry'],
    official: false,
    version: 'v1.5.0',
    addedAt: '2026-07-02',
    featured: false,
    capabilities: [],
    permissions: [
      { resource: 'Pods', access: 'Read-only' },
      { resource: 'Nodes', access: 'Read-only' },
    ],
    features: [
      {
        title: 'One collector, all signals',
        body: 'Collect metrics, logs, traces and profiles with a single agent and a unified configuration.',
      },
    ],
    documentationLinks: [{ label: 'Documentation', url: 'https://grafana.com/docs/alloy' }],
  },
  {
    name: 'cloudnativepg',
    displayName: 'CloudNativePG',
    tagline: 'Production-grade PostgreSQL clusters, the Kubernetes-native way.',
    description:
      'CloudNativePG runs highly available PostgreSQL clusters with streaming replication, automated failover and continuous backup to object storage. Manage your databases declaratively with Cluster resources.',
    vendor: 'Fundament',
    icon: 'cloudnativepg',
    category: 'Database',
    tags: ['database', 'postgres', 'storage'],
    official: true,
    version: 'v1.24.1',
    addedAt: '2026-01-28',
    featured: true,
    capabilities: [],
    permissions: [
      { resource: 'Clusters', access: 'Read and write' },
      { resource: 'Secrets', access: 'Read and write' },
      { resource: 'PersistentVolumeClaims', access: 'Read and write' },
    ],
    features: [
      {
        title: 'Automated failover',
        body: 'A failed primary is detected and a replica is promoted automatically, minimising downtime.',
      },
      {
        title: 'Continuous backup',
        body: 'Base backups and WAL archiving to object storage enable point-in-time recovery.',
      },
    ],
    documentationLinks: [{ label: 'Documentation', url: 'https://cloudnative-pg.io/docs' }],
  },
  {
    name: 'eck-operator',
    displayName: 'Elastic Cloud on Kubernetes',
    tagline: 'Run Elasticsearch and Kibana with the official ECK operator.',
    description:
      'The ECK operator manages Elasticsearch, Kibana and related Elastic Stack components on Kubernetes, handling provisioning, scaling, upgrades and secure-by-default configuration.',
    vendor: 'Elastic',
    icon: 'eck-operator',
    category: 'Database',
    tags: ['search', 'database', 'elastic'],
    official: false,
    version: 'v2.14.0',
    addedAt: '2026-05-09',
    featured: false,
    capabilities: [],
    permissions: [
      { resource: 'Elasticsearch & Kibana', access: 'Read and write' },
      { resource: 'Secrets', access: 'Read and write' },
    ],
    features: [
      {
        title: 'Secure by default',
        body: 'TLS and authentication are configured out of the box for every managed Elastic Stack deployment.',
      },
    ],
    documentationLinks: [
      { label: 'Documentation', url: 'https://www.elastic.co/guide/en/cloud-on-k8s' },
    ],
  },
  {
    name: 'keycloak',
    displayName: 'Keycloak',
    tagline: 'Open-source identity and access management.',
    description:
      'Keycloak provides single sign-on, identity brokering and user federation for your applications. Supports OpenID Connect and SAML, with fine-grained authorization backed by a managed instance.',
    vendor: 'Fundament',
    icon: 'keycloak',
    category: 'Security',
    tags: ['identity', 'sso', 'security'],
    official: true,
    version: 'v26.0.0',
    addedAt: '2026-02-19',
    featured: false,
    capabilities: ['internet_access'],
    permissions: [
      { resource: 'Secrets', access: 'Read and write' },
      { resource: 'Services & Ingresses', access: 'Read and write' },
    ],
    features: [
      {
        title: 'Single sign-on',
        body: 'Give your users one login across all your applications with OIDC and SAML support.',
      },
    ],
    documentationLinks: [{ label: 'Documentation', url: 'https://www.keycloak.org/documentation' }],
  },
  {
    name: 'pinniped',
    displayName: 'Pinniped',
    tagline: 'Consistent cluster authentication from your existing identity provider.',
    description:
      'Pinniped lets users log in to your clusters with credentials from an external OIDC or LDAP identity provider, giving platform teams a single, consistent authentication experience across clusters.',
    vendor: 'Fundament',
    icon: 'pinniped',
    category: 'Security',
    tags: ['authentication', 'security', 'identity'],
    official: true,
    version: 'v0.36.0',
    addedAt: '2026-06-30',
    featured: false,
    capabilities: ['internet_access'],
    permissions: [{ resource: 'TokenReviews', access: 'Read and write' }],
    features: [
      {
        title: 'Bring your own IdP',
        body: 'Authenticate cluster users against the identity provider you already run, with no shared static credentials.',
      },
    ],
    documentationLinks: [{ label: 'Documentation', url: 'https://pinniped.dev/docs' }],
  },
  {
    name: 'sealed-secrets',
    displayName: 'Sealed Secrets',
    tagline: 'Encrypt secrets so they can safely live in Git.',
    description:
      'Sealed Secrets encrypts Kubernetes Secrets into SealedSecret resources that are safe to store in version control. A controller decrypts them in-cluster at apply time, so plaintext never leaves your cluster.',
    vendor: 'Fundament',
    icon: 'sealed-secrets',
    category: 'Security',
    tags: ['secrets', 'gitops', 'security'],
    official: true,
    version: 'v0.27.1',
    addedAt: '2026-04-01',
    featured: false,
    capabilities: [],
    permissions: [{ resource: 'Secrets', access: 'Read and write' }],
    features: [
      {
        title: 'GitOps-friendly secrets',
        body: 'Commit encrypted secrets to your repository and let the controller decrypt them safely in-cluster.',
      },
    ],
    documentationLinks: [
      { label: 'Documentation', url: 'https://github.com/bitnami-labs/sealed-secrets' },
    ],
  },
  {
    name: 'openfsc',
    displayName: 'OpenFSC Gateway',
    tagline: 'Federated service connectivity for Dutch government systems.',
    description:
      'OpenFSC provides standardised, secure connectivity between government services over the Federated Service Connectivity protocol, handling authentication, authorization and audit logging at the edge.',
    vendor: 'RINIS',
    icon: 'openfsc',
    category: 'Networking',
    tags: ['government', 'connectivity', 'networking'],
    official: true,
    version: 'v0.9.0',
    addedAt: '2026-07-08',
    featured: false,
    capabilities: ['internet_access'],
    permissions: [
      { resource: 'Services & Ingresses', access: 'Read and write' },
      { resource: 'Secrets', access: 'Read-only' },
    ],
    features: [
      {
        title: 'Standards-based interconnect',
        body: 'Connect to other government services using the FSC protocol with built-in audit logging.',
      },
    ],
    documentationLinks: [{ label: 'Documentation', url: 'https://example.gov/openfsc/docs' }],
  },
];

const CATEGORIES: Category[] = [
  { id: 'Database', name: 'Database' },
  { id: 'Networking', name: 'Networking' },
  { id: 'Observability', name: 'Observability' },
  { id: 'Security', name: 'Security' },
];

@Injectable({ providedIn: 'root' })
export default class MarketplaceService {
  private readonly plugins = PLUGINS;

  private readonly categories = CATEGORIES;

  listPlugins(): Promise<MarketplacePlugin[]> {
    return Promise.resolve(this.plugins.map((plugin) => ({ ...plugin })));
  }

  getPlugin(name: string): Promise<MarketplacePlugin | null> {
    const plugin = this.plugins.find((p) => p.name === name);
    return Promise.resolve(plugin ? { ...plugin } : null);
  }

  listCategories(): Promise<Category[]> {
    return Promise.resolve(this.categories.map((category) => ({ ...category })));
  }
}
