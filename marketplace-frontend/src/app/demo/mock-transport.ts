// Demo-only in-memory ConnectRPC transports for the static walkthrough build.
// Every RPC the storefront and the developer surface make is answered from
// handwritten fixtures — no network, no backend, so the bundle can be served
// from the console demo's own origin under a `connect-src 'self'` policy.
import { create } from '@bufbuild/protobuf';
import { Transport, createRouterTransport } from '@connectrpc/connect';
import {
  CatalogService,
  ListPluginsResponseSchema,
  GetPluginResponseSchema,
  ListPluginVersionsResponseSchema,
  ListCategoriesResponseSchema,
  ListPublishersResponseSchema,
} from '../../generated/catalog/v1/catalog_pb';
import {
  PublicationService,
  ListPluginsResponseSchema as ListAuthoredPluginsResponseSchema,
  GetPluginResponseSchema as GetAuthoredPluginResponseSchema,
  ListPluginVersionsResponseSchema as ListAuthoredVersionsResponseSchema,
  GetPluginVersionResponseSchema,
  SubmitPluginVersionResponseSchema,
  WithdrawPluginVersionResponseSchema,
  UpdatePluginResponseSchema,
  ListCategoriesResponseSchema as ListRegistryCategoriesResponseSchema,
} from '../../generated/registry/v1/publication_pb';
import { ListingDraftSchema, SubmissionStatus } from '../../generated/marketplace/v1/common_pb';
import * as fx from './fixtures';

// Artificial latency so the app's loading and skeleton states are visible while
// presenting, matching the console demo's transport.
const LATENCY_MS = 260;
const delay = (ms = LATENCY_MS) =>
  new Promise((resolve) => {
    setTimeout(resolve, ms);
  });

/** The anonymous storefront API: catalog.v1.CatalogService. */
export function createDemoCatalogTransport(): Transport {
  return createRouterTransport((router) => {
    router.service(CatalogService, {
      // Filtering is done client-side by the storefront (the real catalog is
      // small enough that ListPlugins returns everything), so the request's
      // query and category are deliberately ignored here too.
      listPlugins: async () => {
        await delay();
        return create(ListPluginsResponseSchema, { plugins: fx.plugins });
      },
      getPlugin: async (req) => {
        await delay();
        return create(GetPluginResponseSchema, { plugin: fx.pluginDetails(req.pluginId) });
      },
      listPluginVersions: async (req) => {
        await delay(80);
        return create(ListPluginVersionsResponseSchema, {
          versions: fx.pluginVersions(req.pluginId),
        });
      },
      listCategories: async () => {
        await delay(80);
        return create(ListCategoriesResponseSchema, { categories: fx.categories });
      },
      listPublishers: async () => {
        await delay(80);
        return create(ListPublishersResponseSchema, { publishers: fx.publishers });
      },
    });
  });
}

// A data URI for an uploaded logo; the registry sniffs PNG the same way.
function logoDataUri(content: Uint8Array): string {
  if (content.length === 0) return '';
  const isPng = content[0] === 0x89 && content[1] === 0x50;
  const binary = Array.from(content, (byte) => String.fromCharCode(byte)).join('');
  return `data:${isPng ? 'image/png' : 'image/svg+xml'};base64,${btoa(binary)}`;
}

/** The developer's publishing API: registry.v1.PublicationService. */
export function createDemoRegistryTransport(): Transport {
  return createRouterTransport((router) => {
    router.service(PublicationService, {
      listPlugins: async () => {
        await delay();
        return create(ListAuthoredPluginsResponseSchema, { plugins: fx.authoredPlugins });
      },
      getPlugin: async (req) => {
        await delay();
        return create(GetAuthoredPluginResponseSchema, {
          plugin: fx.authoredPlugins.find((plugin) => plugin.id === req.pluginId),
        });
      },
      listPluginVersions: async (req) => {
        await delay(80);
        return create(ListAuthoredVersionsResponseSchema, {
          versions: fx.versionsForPlugin(req.pluginId),
        });
      },
      getPluginVersion: async (req) => {
        await delay(80);
        return create(GetPluginVersionResponseSchema, {
          version: fx.findVersion(req.pluginVersionId),
        });
      },
      // Submit and withdraw mutate the fixture in place, so the status tracker
      // keeps the new state for the rest of the slide.
      submitPluginVersion: async (req) => {
        await delay(400);
        const version = fx.findVersion(req.pluginVersionId);
        if (version) version.status = SubmissionStatus.PENDING;
        return create(SubmitPluginVersionResponseSchema, { version });
      },
      withdrawPluginVersion: async (req) => {
        await delay(400);
        const version = fx.findVersion(req.pluginVersionId);
        if (version) version.status = SubmissionStatus.WITHDRAWN;
        return create(WithdrawPluginVersionResponseSchema, { version });
      },
      // Mutates the fixture in place, like submit and withdraw, so the saved
      // listing survives navigating back to it. As on the registry, visibility
      // applies at once and the rest waits for review, which the demo has no
      // reviewer for.
      updatePlugin: async (req) => {
        await delay(400);
        const plugin = fx.authoredPlugins.find((candidate) => candidate.id === req.pluginId);
        if (plugin) {
          Object.assign(plugin, {
            visibility: req.visibility,
            allowedOrganizationIds: req.allowedOrganizationIds,
            pendingListing: create(ListingDraftSchema, {
              description: req.description,
              categoryIds: req.categoryIds,
              image: req.image,
              features: req.features,
              documentationLinks: req.documentationLinks,
              authorName: req.authorName,
              authorUrl: req.authorUrl,
              logo: logoDataUri(req.logo),
              logoDark: logoDataUri(req.logoDark),
            }),
          });
        }
        return create(UpdatePluginResponseSchema, { plugin });
      },
      listCategories: async () => {
        await delay(80);
        return create(ListRegistryCategoriesResponseSchema, { categories: fx.categories });
      },
    });
  });
}
