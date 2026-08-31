import { Injectable, inject } from '@angular/core';
import { firstValueFrom } from 'rxjs';
import { CATALOG_CLIENT } from '../../connect/tokens';
import toIsoDate from '../../connect/timestamp';
import { PluginLabel as ProtoPluginLabel } from '../../generated/catalog/v1/common_pb';
import {
  type PluginSummary,
  type PluginDetails,
  type PublishedVersion,
} from '../../generated/catalog/v1/catalog_pb';
import {
  type Category as ProtoCategory,
  type DocumentationLink as ProtoDocumentationLink,
  type PluginPermission as ProtoPluginPermission,
  type FeatureBlock as ProtoFeatureBlock,
} from '../../generated/marketplace/v1/common_pb';

// Public marketplace catalog, backed by catalog.v1.CatalogService. The service
// is anonymous and internet-facing: it returns only PUBLIC listings that have a
// published version, so there is no visibility field to honour here.
//
// The catalog is small and pagination is deferred (FUN-20), so listPlugins()
// fetches the whole catalog and the index filters it client-side. That is also
// what the sidebar counts and the "search all categories" escape hatch need.

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

// Trust and support labels a listing can carry. `core` and `rijksoverheid` say
// where a plugin comes from, `support-9-to-17` what support it ships with, so a
// plugin can hold several at once.
export type PluginLabel = 'core' | 'rijksoverheid' | 'support-9-to-17';

// Mirrors catalog.v1.PluginSummary: what a card or a results grid needs.
export interface MarketplacePluginSummary {
  id: string; // UUID, used in URLs
  name: string; // stable slug, unique per publisher
  displayName: string;
  tagline: string; // one-line summary shown on cards
  vendor: string;
  icon: string; // base name under /img/plugins/<icon>.svg
  image: string; // listing artwork; not rendered yet, see plugin-icon fallback
  categoryIds: string[];
  categoryName: string; // first category, resolved for display
  tags: string[];
  labels: PluginLabel[];
  addedAt: string; // ISO date, used to sort "recently added"; '' when unpublished
}

// Mirrors catalog.v1.PluginDetails: everything the detail page adds on top.
export interface MarketplacePluginDetails extends MarketplacePluginSummary {
  description: string; // longer paragraph shown on the detail page
  version: string; // latest published version, resolved from latestVersionId
  // Declared capabilities (e.g. internet access) the plugin needs.
  capabilities: string[];
  // RBAC-style permissions, shown on the detail page.
  permissions: PluginPermission[];
  features: FeatureBlock[];
  documentationLinks: DocumentationLink[];
}

export interface Category {
  id: string; // UUID, matches MarketplacePluginSummary.categoryIds
  name: string;
}

// The fields PluginSummary and PluginDetails share. Generated messages are
// branded with their own type name, so a PluginDetails is not assignable to a
// PluginSummary; the structural subset is what the shared mapper takes.
type ListingFields = Pick<
  PluginSummary,
  | 'id'
  | 'name'
  | 'displayName'
  | 'descriptionShort'
  | 'organizationId'
  | 'image'
  | 'categoryIds'
  | 'tags'
  | 'labels'
  | 'published'
>;

// PLUGIN_LABEL_UNSPECIFIED has no badge, so it maps to nothing and is dropped.
const PLUGIN_LABELS: Partial<Record<ProtoPluginLabel, PluginLabel>> = {
  [ProtoPluginLabel.CORE]: 'core',
  [ProtoPluginLabel.RIJKSOVERHEID]: 'rijksoverheid',
  [ProtoPluginLabel.SUPPORT_9_TO_17]: 'support-9-to-17',
};

@Injectable({ providedIn: 'root' })
export default class MarketplaceService {
  private readonly client = inject(CATALOG_CLIENT);

  // Publishers and categories are small, stable lookup tables that every
  // mapping needs. Memoized on the root singleton so index -> detail costs two
  // RPCs rather than four.
  private publishers?: Promise<Map<string, string>>;

  private categories?: Promise<Category[]>;

  async listPlugins(): Promise<MarketplacePluginSummary[]> {
    const [response, publishers, categories] = await Promise.all([
      firstValueFrom(this.client.listPlugins({})),
      this.loadPublishers(),
      this.loadCategories(),
    ]);
    const categoryNames = MarketplaceService.byId(categories);
    return response.plugins.map((plugin) =>
      MarketplaceService.toSummary(plugin, publishers, categoryNames),
    );
  }

  async getPlugin(id: string): Promise<MarketplacePluginDetails | null> {
    const [response, versions, publishers, categories] = await Promise.all([
      firstValueFrom(this.client.getPlugin({ pluginId: id })),
      firstValueFrom(this.client.listPluginVersions({ pluginId: id })),
      this.loadPublishers(),
      this.loadCategories(),
    ]);
    const plugin = response.plugin;
    if (!plugin) return null;
    return MarketplaceService.toDetails(
      plugin,
      versions.versions,
      publishers,
      MarketplaceService.byId(categories),
    );
  }

  listCategories(): Promise<Category[]> {
    return this.loadCategories();
  }

  private loadPublishers(): Promise<Map<string, string>> {
    this.publishers ??= firstValueFrom(this.client.listPublishers({})).then(
      (response) =>
        new Map(response.publishers.map((publisher) => [publisher.id, publisher.displayName])),
    );
    return this.publishers;
  }

  private loadCategories(): Promise<Category[]> {
    this.categories ??= firstValueFrom(this.client.listCategories({})).then((response) =>
      response.categories.map((category: ProtoCategory) => ({
        id: category.id,
        name: category.name,
      })),
    );
    return this.categories;
  }

  private static byId(categories: Category[]): Map<string, string> {
    return new Map(categories.map((category) => [category.id, category.name]));
  }

  private static toSummary(
    plugin: ListingFields,
    publishers: Map<string, string>,
    categories: Map<string, string>,
  ): MarketplacePluginSummary {
    return {
      id: plugin.id,
      name: plugin.name,
      displayName: plugin.displayName,
      tagline: plugin.descriptionShort,
      // Falls back to the raw organization id: a listing whose publisher is not
      // in ListPublishers should still render.
      vendor: publishers.get(plugin.organizationId) ?? plugin.organizationId,
      icon: plugin.name,
      image: plugin.image,
      categoryIds: plugin.categoryIds,
      categoryName: MarketplaceService.categoryName(plugin.categoryIds, categories),
      tags: plugin.tags,
      labels: MarketplaceService.toLabels(plugin.labels),
      addedAt: toIsoDate(plugin.published),
    };
  }

  private static toDetails(
    plugin: PluginDetails,
    versions: PublishedVersion[],
    publishers: Map<string, string>,
    categories: Map<string, string>,
  ): MarketplacePluginDetails {
    return {
      ...MarketplaceService.toSummary(plugin, publishers, categories),
      description: plugin.description,
      version: MarketplaceService.latestVersion(plugin.latestVersionId, versions),
      capabilities: plugin.capabilities,
      permissions: plugin.permissions.map((permission: ProtoPluginPermission) => ({
        resource: permission.resource,
        access: permission.access,
      })),
      features: plugin.features.map((feature: ProtoFeatureBlock) => ({
        title: feature.title,
        body: feature.body,
      })),
      // `title` labels the group a link appears under, which this page does not
      // render; url_name is the link text, so it wins where both are set.
      documentationLinks: plugin.documentationLinks.map((link: ProtoDocumentationLink) => ({
        label: link.urlName || link.title,
        url: link.url,
      })),
    };
  }

  // Only the first category is shown; a listing can carry several, and all of
  // them stay available through categoryIds for filtering.
  private static categoryName(categoryIds: string[], categories: Map<string, string>): string {
    const first = categoryIds[0];
    return first ? (categories.get(first) ?? '') : '';
  }

  private static toLabels(labels: ProtoPluginLabel[]): PluginLabel[] {
    return labels
      .map((label) => PLUGIN_LABELS[label])
      .filter((label): label is PluginLabel => label !== undefined);
  }

  private static latestVersion(latestVersionId: string, versions: PublishedVersion[]): string {
    const latest =
      versions.find((version) => version.id === latestVersionId) ?? versions[0] ?? null;
    return latest?.version ?? '';
  }
}
