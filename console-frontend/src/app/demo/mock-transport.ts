// Demo-only in-memory ConnectRPC transport for the static walkthrough build.
// Redirects every RPC to handwritten fixtures — no network, no backend.
import { create } from '@bufbuild/protobuf';
import { timestampFromDate } from '@bufbuild/protobuf/wkt';
import { Transport, createRouterTransport } from '@connectrpc/connect';
import {
  OrganizationService,
  ListOrganizationsResponseSchema,
  GetOrganizationResponseSchema,
  GetOrganizationLimitsResponseSchema,
} from '../../generated/v1/organization_pb';
import {
  ClusterService,
  ListClustersResponseSchema,
  GetClusterResponseSchema,
  ListNodePoolsResponseSchema,
  GetClusterActivityResponseSchema,
  CreateClusterResponseSchema,
  ListRegionsResponseSchema,
  ListClustersResponse_ClusterSummarySchema,
  ClusterDetailsSchema,
} from '../../generated/v1/cluster_pb';
import {
  NamespaceService,
  ListClusterNamespacesResponseSchema,
  ListProjectNamespacesResponseSchema,
  CreateNamespaceResponseSchema,
  NamespaceSchema,
} from '../../generated/v1/namespace_pb';
import {
  ProjectService,
  ListProjectsResponseSchema,
  GetProjectResponseSchema,
  ListProjectMembersResponseSchema,
  GetProjectLimitsResponseSchema,
} from '../../generated/v1/project_pb';
import { MemberService, ListMembersResponseSchema } from '../../generated/v1/member_pb';
import { InviteService, ListInvitationsResponseSchema } from '../../generated/v1/invite_pb';
import {
  PluginService,
  ListPluginsResponseSchema,
  ListPresetsResponseSchema,
} from '../../generated/v1/plugin_pb';
import { APIKeyService, ListAPIKeysResponseSchema } from '../../generated/v1/apikey_pb';
import {
  CatalogService,
  ListPluginsResponseSchema as CatalogListPluginsResponseSchema,
  PluginSummarySchema as CatalogPluginSummarySchema,
  PluginDetailsSchema as CatalogPluginDetailsSchema,
  GetPluginResponseSchema as CatalogGetPluginResponseSchema,
  ListCategoriesResponseSchema as CatalogListCategoriesResponseSchema,
  ListPublishersResponseSchema as CatalogListPublishersResponseSchema,
  ListPluginVersionsResponseSchema as CatalogListPluginVersionsResponseSchema,
  PublishedVersionSchema,
} from '../../generated/catalog/v1/catalog_pb';
import {
  CategorySchema,
  PublisherSchema,
  DocumentationLinkSchema as MarketplaceDocumentationLinkSchema,
} from '../../generated/marketplace/v1/common_pb';
import { AuthnService, GetUserInfoResponseSchema } from '../../generated/authn/v1/authn_pb';
import {
  MetricsService,
  GetOrgWorkloadMetricsResponseSchema,
  GetProjectWorkloadMetricsResponseSchema,
  GetClusterWorkloadMetricsResponseSchema,
  GetWorkloadTimeSeriesResponseSchema,
  StreamWorkloadMetricsResponseSchema,
} from '../../generated/v1/metrics_pb';
import { ClusterStatus } from '../../generated/v1/common_pb';
import * as fx from './fixtures';

// Artificial latency so the app's loading/skeleton states are visible while presenting.
const LATENCY_MS = 260;
const delay = (ms = LATENCY_MS) =>
  new Promise((resolve) => {
    setTimeout(resolve, ms);
  });

// The seeded 'system' organization publishes every demo plugin; catalog.v1
// returns its id and leaves the name to ListPublishers.
const DEMO_PUBLISHER_ID = 'org-system';

// The categories the fixtures actually use, deduplicated — the catalog serves
// the whole list, not one per plugin.
const demoCategories = () => {
  const byId = new Map<string, { id: string; name: string }>();
  fx.plugins.forEach((p) =>
    p.categories.forEach((c) => byId.set(c.id, { id: c.id, name: c.name })),
  );
  return [...byId.values()];
};

export default function createDemoTransport(): Transport {
  return createRouterTransport((router) => {
    router.service(AuthnService, {
      getUserInfo: async () => {
        await delay(80);
        return create(GetUserInfoResponseSchema, { user: fx.demoUser });
      },
    });

    router.service(OrganizationService, {
      listOrganizations: async () => {
        await delay();
        return create(ListOrganizationsResponseSchema, { organizations: [fx.organization] });
      },
      getOrganization: async () => {
        await delay();
        return create(GetOrganizationResponseSchema, { organization: fx.organization });
      },
      getOrganizationLimits: async () => {
        await delay();
        return create(GetOrganizationLimitsResponseSchema, {
          limits: fx.organizationLimits,
          defaults: fx.platformOrganizationLimits,
        });
      },
    });

    router.service(ClusterService, {
      listClusters: async () => {
        await delay();
        return create(ListClustersResponseSchema, { clusters: fx.clusterSummaries });
      },
      getCluster: async (req) => {
        await delay();
        const details = fx.clusterDetails.get(req.clusterId);
        return create(GetClusterResponseSchema, { cluster: details });
      },
      getClusterByName: async (req) => {
        await delay();
        const details = [...fx.clusterDetails.values()].find((c) => c.name === req.name);
        return create(GetClusterResponseSchema, { cluster: details });
      },
      listNodePools: async (req) => {
        await delay();
        return create(ListNodePoolsResponseSchema, {
          nodePools: fx.nodePoolsByCluster.get(req.clusterId) ?? [],
        });
      },
      listRegions: async () => {
        await delay();
        return create(ListRegionsResponseSchema, { regions: fx.regions });
      },
      getClusterActivity: async () => {
        await delay();
        return create(GetClusterActivityResponseSchema, { events: fx.clusterActivity });
      },
      createCluster: async (req, ctx) => {
        await delay(500);
        // Same as createNamespace: the caller polls until this header says the
        // work is done, so without it the form sits on a cluster that exists.
        ctx.responseHeader.set('Idempotency-Status', 'completed');
        const id = `cl-${req.name}`;
        // Append so the cluster list reflects the form result on the next visit.
        if (!fx.clusterSummaries.some((c) => c.id === id)) {
          fx.clusterSummaries.push(
            create(ListClustersResponse_ClusterSummarySchema, {
              id,
              name: req.name,
              status: ClusterStatus.PROVISIONING,
              region: req.region || 'local',
              projectCount: 0,
              nodePoolCount: 1,
            }),
          );
          fx.clusterDetails.set(
            id,
            create(ClusterDetailsSchema, {
              id,
              name: req.name,
              region: req.region || 'local',
              kubernetesVersion: req.kubernetesVersion || '1.34.0',
              status: ClusterStatus.PROVISIONING,
            }),
          );
        }
        return create(CreateClusterResponseSchema, { clusterId: id });
      },
    });

    router.service(NamespaceService, {
      listClusterNamespaces: async (req) => {
        await delay();
        return create(ListClusterNamespacesResponseSchema, {
          namespaces: fx.namespaces.filter((n) => n.clusterId === req.clusterId),
        });
      },
      listProjectNamespaces: async (req) => {
        await delay();
        return create(ListProjectNamespacesResponseSchema, {
          namespaces: fx.namespaces.filter((n) => n.projectId === req.projectId),
        });
      },
      createNamespace: async (req, ctx) => {
        await delay();
        // The caller polls until this header says the work is done (see
        // withIdempotency). Without it a create in the demo never resolves and
        // the sheet stays open on something that already happened.
        ctx.responseHeader.set('Idempotency-Status', 'completed');
        const id = `ns-${req.name}`;
        // Append so the list shows what was just created, the way the cluster
        // form does. The access handed out with it lives in the mock role
        // bindings and needs the namespace to exist to be visible at all.
        if (!fx.namespaces.some((n) => n.id === id)) {
          const project = fx.projects.find((p) => p.id === req.projectId);
          fx.namespaces.push(
            create(NamespaceSchema, {
              id,
              name: req.name,
              projectId: req.projectId,
              clusterId: project?.clusterId ?? 'cl-production',
              created: timestampFromDate(new Date()),
            }),
          );
        }
        return create(CreateNamespaceResponseSchema, { namespaceId: id });
      },
    });

    router.service(ProjectService, {
      listProjects: async (req) => {
        await delay();
        const projects = req.clusterId
          ? fx.projects.filter((p) => p.clusterId === req.clusterId)
          : fx.projects;
        return create(ListProjectsResponseSchema, { projects });
      },
      getProject: async (req) => {
        await delay();
        return create(GetProjectResponseSchema, {
          project: fx.projects.find((p) => p.id === req.projectId),
        });
      },
      getProjectByName: async (req) => {
        await delay();
        return create(GetProjectResponseSchema, {
          project: fx.projects.find((p) => p.name === req.name),
        });
      },
      listProjectMembers: async (req) => {
        await delay();
        return create(ListProjectMembersResponseSchema, {
          members: fx.projectMembersByProject.get(req.projectId) ?? [],
        });
      },
      getProjectLimits: async () => {
        await delay();
        return create(GetProjectLimitsResponseSchema, {
          limits: fx.projectLimits,
          defaults: fx.platformProjectLimits,
        });
      },
    });

    router.service(MemberService, {
      listMembers: async () => {
        await delay();
        return create(ListMembersResponseSchema, { members: fx.members });
      },
    });

    router.service(InviteService, {
      listInvitations: async () => {
        await delay(80);
        return create(ListInvitationsResponseSchema, { invitations: [] });
      },
    });

    // What organization.v1 still answers. listPlugins stays because the
    // cluster-side views (cluster-plugins, cluster-details, shared-plugins-form)
    // have not moved to the catalog; presets have no catalog equivalent at all.
    router.service(PluginService, {
      listPlugins: async () => {
        await delay();
        return create(ListPluginsResponseSchema, { plugins: fx.plugins });
      },
      listPresets: async () => {
        await delay(80);
        return create(ListPresetsResponseSchema, { presets: fx.presets });
      },
    });

    router.service(CatalogService, {
      listPlugins: async () => {
        await delay();
        return create(CatalogListPluginsResponseSchema, {
          plugins: fx.plugins.map((p) =>
            create(CatalogPluginSummarySchema, {
              id: p.id,
              name: p.name,
              displayName: p.displayName,
              descriptionShort: p.descriptionShort,
              organizationId: DEMO_PUBLISHER_ID,
              image: p.image,
              categoryIds: p.categories.map((c) => c.id),
              tags: p.tags.map((t) => t.name),
            }),
          ),
        });
      },
      listCategories: async () => {
        await delay(80);
        return create(CatalogListCategoriesResponseSchema, {
          categories: demoCategories().map((c) => create(CategorySchema, c)),
        });
      },
      listPublishers: async () => {
        await delay(80);
        return create(CatalogListPublishersResponseSchema, {
          publishers: [
            create(PublisherSchema, {
              id: DEMO_PUBLISHER_ID,
              name: 'system',
              displayName: 'System',
            }),
          ],
        });
      },
      getPlugin: async (req) => {
        await delay();
        const detail = fx.pluginDetail(req.pluginId);
        if (!detail) {
          return create(CatalogGetPluginResponseSchema, {});
        }
        return create(CatalogGetPluginResponseSchema, {
          plugin: create(CatalogPluginDetailsSchema, {
            id: detail.id,
            name: detail.name,
            displayName: detail.displayName,
            description: detail.description,
            descriptionShort: detail.descriptionShort,
            organizationId: DEMO_PUBLISHER_ID,
            categoryIds: detail.categories.map((c) => c.id),
            tags: detail.tags.map((t) => t.name),
            authorName: detail.author?.name ?? '',
            authorUrl: detail.author?.url ?? '',
            repositoryUrl: detail.repositoryUrl,
            // Same fields, different package: organization.v1's links cannot be
            // handed to a marketplace.v1 message as-is.
            documentationLinks: detail.documentationLinks.map((l) =>
              create(MarketplaceDocumentationLinkSchema, {
                id: l.id,
                title: l.title,
                urlName: l.urlName,
                url: l.url,
              }),
            ),
          }),
        });
      },
      // The install modal's version picker. Left unanswered it errors, and the modal
      // shows "Couldn't load versions" instead of letting the install slide run.
      listPluginVersions: async (req) => {
        await delay(80);
        return create(CatalogListPluginVersionsResponseSchema, {
          versions: fx.pluginDefinitionVersions(req.pluginId).map((d) =>
            create(PublishedVersionSchema, {
              id: `ver-${req.pluginId}-${d.version}`,
              version: d.version,
              definitionHash: d.hash,
            }),
          ),
        });
      },
    });

    // Metrics, so the charts have something to draw. One snapshot per stream:
    // the demo is about the shape of the page, not about watching it tick.
    const timeSeries = (windowSeconds: number, stepSeconds: number) => {
      const step = stepSeconds || 300;
      const span = windowSeconds || 7 * 24 * 3600;
      const count = Math.min(240, Math.max(12, Math.round(span / step)));
      const now = Date.now();
      const toSamples = (points: { timestamp: Date; value: number }[]) =>
        points.map((point) => ({
          timestamp: timestampFromDate(point.timestamp),
          value: point.value,
        }));
      return create(GetWorkloadTimeSeriesResponseSchema, {
        cpuCores: toSamples(fx.metricSeries(count, step, 2.4, 0.6, 0.1, now)),
        memoryGib: toSamples(fx.metricSeries(count, step, 12.8, 2.4, 0.4, now)),
        podCount: toSamples(fx.metricSeries(count, step, 28, 4, 0.7, now)),
        networkReceiveMbS: toSamples(fx.metricSeries(count, step, 1.8, 0.7, 0.2, now)),
        networkTransmitMbS: toSamples(fx.metricSeries(count, step, 0.9, 0.4, 0.9, now)),
      });
    };

    const snapshot = (level: 'org' | 'cluster' | 'project', windowSeconds = 0, stepSeconds = 0) =>
      create(StreamWorkloadMetricsResponseSchema, {
        totals:
          level === 'project'
            ? {
                cpu: { used: 1.4, total: 4, unit: 'cores' },
                memory: { used: 6.1, total: 16, unit: 'GiB' },
                pods: { used: 14, total: 55, unit: 'pods' },
              }
            : {
                cpu: { used: 3.3, total: 12, unit: 'cores' },
                memory: { used: 16.4, total: 48, unit: 'GiB' },
                pods: { used: 37, total: 165, unit: 'pods' },
              },
        clusters: level === 'org' ? fx.clusterUsage : [],
        nodes: level === 'cluster' ? fx.nodeUsage : [],
        namespaces: level === 'project' ? fx.namespaceMetrics.slice(0, 1) : fx.namespaceMetrics,
        timeSeries: timeSeries(windowSeconds, stepSeconds),
        refreshedAt: timestampFromDate(new Date()),
      });

    router.service(MetricsService, {
      getOrgWorkloadMetrics: async () => {
        await delay(80);
        return create(GetOrgWorkloadMetricsResponseSchema, {
          clusters: fx.clusterUsage,
          namespaces: fx.namespaceMetrics,
        });
      },
      getClusterWorkloadMetrics: async () => {
        await delay();
        return create(GetClusterWorkloadMetricsResponseSchema, {
          nodes: fx.nodeUsage,
          namespaces: fx.namespaceMetrics,
        });
      },
      getProjectWorkloadMetrics: async () => {
        await delay();
        return create(GetProjectWorkloadMetricsResponseSchema, {
          namespaces: fx.namespaceMetrics.slice(0, 1),
        });
      },
      getOrgWorkloadTimeSeries: async () => {
        await delay();
        return timeSeries(0, 0);
      },
      getClusterWorkloadTimeSeries: async () => {
        await delay();
        return timeSeries(0, 0);
      },
      getProjectWorkloadTimeSeries: async () => {
        await delay();
        return timeSeries(0, 0);
      },
      async *streamOrgWorkloadMetrics(req) {
        await delay();
        yield snapshot('org', req.windowSeconds, req.stepSeconds);
      },
      async *streamClusterWorkloadMetrics(req) {
        await delay();
        yield snapshot('cluster', req.windowSeconds, req.stepSeconds);
      },
      async *streamProjectWorkloadMetrics(req) {
        await delay();
        yield snapshot('project', req.windowSeconds, req.stepSeconds);
      },
    });

    router.service(APIKeyService, {
      listAPIKeys: async () => {
        await delay(80);
        return create(ListAPIKeysResponseSchema, { apiKeys: [] });
      },
    });
  });
}
