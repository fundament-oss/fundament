// Demo-only in-memory ConnectRPC transport for the static walkthrough build.
// Redirects every RPC to handwritten fixtures — no network, no backend.
import { create } from '@bufbuild/protobuf';
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
  ListPresetsResponseSchema as CatalogListPresetsResponseSchema,
  PublishedVersionSchema,
} from '../../generated/catalog/v1/catalog_pb';
import {
  CategorySchema,
  PresetSchema,
  PublisherSchema,
  DocumentationLinkSchema as MarketplaceDocumentationLinkSchema,
} from '../../generated/marketplace/v1/common_pb';
import { AuthnService, GetUserInfoResponseSchema } from '../../generated/authn/v1/authn_pb';
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
          defaults: fx.organizationLimits,
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
      createCluster: async (req) => {
        await delay(500);
        const id = `cl-${req.name}`;
        // Append so the cluster list reflects the wizard result on the next visit.
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
          defaults: fx.projectLimits,
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

    // What organization.v1 still answers: the cluster-side views
    // (cluster-plugins, cluster-details, shared-plugins-form) have not moved to
    // the catalog, and shared-plugins-form still reads presets from here too.
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
      listPresets: async () => {
        await delay(80);
        return create(CatalogListPresetsResponseSchema, {
          presets: fx.presets.map((p) =>
            create(PresetSchema, {
              id: p.id,
              name: p.name,
              description: p.description,
              pluginIds: p.pluginIds,
            }),
          ),
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

    router.service(APIKeyService, {
      listAPIKeys: async () => {
        await delay(80);
        return create(ListAPIKeysResponseSchema, { apiKeys: [] });
      },
    });
  });
}
