import { Injectable, inject } from '@angular/core';
import { firstValueFrom } from 'rxjs';
import { PUBLICATION_CLIENT } from '../../connect/tokens';
import toIsoDate, { toEpochMs } from '../../connect/timestamp';
import { type SubmissionStatus, fromProtoStatus } from '../status/submission-status';
import {
  type Plugin as RegistryPlugin,
  type PluginVersion as RegistryPluginVersion,
  PluginVisibility,
} from '../../generated/registry/v1/common_pb';

// The plugin developer's surface, backed by registry.v1.PublicationService.
// Every RPC is scoped to plugins owned by the caller's organization; this is
// the same API `functl plugins push` talks to.

export interface PluginAuthor {
  name: string;
  url: string;
}

export interface PluginVersion {
  id: string; // needed to submit or withdraw this build
  version: string;
  pushedAt: string; // ISO date
  status: SubmissionStatus;
  // Container image the version runs, read out of the pinned manifest. This is
  // a property of the build, not of the listing: registry.v1.Plugin.image is
  // the listing artwork and is a different thing entirely.
  image: string;
  // Reviewer note explaining a changes_requested or rejected decision. It is
  // the only part of a review decision the developer ever sees.
  reviewFeedback?: string;
}

export interface AuthoredPlugin {
  id: string; // UUID, used in URLs and by every version RPC
  name: string; // stable slug, unique within the owning organization
  displayName: string;
  descriptionShort: string;
  description: string;
  version: string; // latest pushed version
  author: PluginAuthor;
  repositoryUrl: string;
  logo: string; // data: URI; empty when the plugin has none
  logoDark: string;
  tags: string[];
  categoryName: string;
  // A listing carries no review state of its own (FUN-20): the reviewed unit is
  // a version, so this is the newest version's status.
  status: SubmissionStatus;
  // Newest first.
  versions: PluginVersion[];
  // Listing edits are saved but wait for a submitted version's approval.
  listingPendingReview: boolean;
}

export interface Category {
  id: string;
  name: string;
}

export interface ListingFeature {
  id: string; // empty for a feature not saved yet
  title: string;
  body: string;
}

export interface ListingLink {
  id: string; // empty for a link not saved yet
  title: string; // the group the link is listed under
  urlName: string; // the link text
  url: string;
}

export type ListingVisibility = 'public' | 'restricted';

// The listing as UpdatePlugin takes it. The fields definition.yaml owns sit in
// `synced`: they change when a version is approved, not here. The editable
// fields are the pending edits when there are any, else the live listing.
export interface PluginListing {
  id: string;
  // Edits are saved but wait for a submitted version to be approved.
  pendingReview: boolean;
  synced: {
    displayName: string;
    descriptionShort: string;
    tags: string[];
    repositoryUrl: string;
    license: string;
  };
  description: string;
  categoryIds: string[];
  image: string;
  features: ListingFeature[];
  documentationLinks: ListingLink[];
  visibility: ListingVisibility;
  allowedOrganizationIds: string[];
  authorName: string;
  authorUrl: string;
  // The listing's own logos as data URIs, shown instead of the version's;
  // empty when it has none.
  logo: string;
  logoDark: string;
  // The newest version's logos, known only while no upload hides them.
  versionLogo: string;
  versionLogoDark: string;
}

// The bytes of a base64 data: URI, which is how logos are read and uploaded.
export function dataUriBytes(uri: string): Uint8Array {
  if (uri === '') return new Uint8Array();
  const binary = atob(uri.slice(uri.indexOf(',') + 1));
  return Uint8Array.from(binary, (char) => char.charCodeAt(0));
}

@Injectable({ providedIn: 'root' })
export default class PluginDevelopmentService {
  private readonly client = inject(PUBLICATION_CLIENT);

  private categories?: Promise<Map<string, string>>;

  async listPlugins(): Promise<AuthoredPlugin[]> {
    const [response, categories] = await Promise.all([
      firstValueFrom(this.client.listPlugins({})),
      this.loadCategories(),
    ]);
    // ListPlugins carries no version state, so the status column costs one
    // ListPluginVersions per row. Bounded by construction: this is one
    // organization's own listings, not the whole catalog. A latest_version_id
    // plus its status on registry.v1.Plugin would remove the fan-out.
    const versions = await Promise.all(
      response.plugins.map((plugin) =>
        firstValueFrom(this.client.listPluginVersions({ pluginId: plugin.id })),
      ),
    );
    return response.plugins.map((plugin, index) =>
      PluginDevelopmentService.toPlugin(plugin, versions[index]?.versions ?? [], categories),
    );
  }

  async getPlugin(id: string): Promise<AuthoredPlugin | null> {
    const [response, versions, categories] = await Promise.all([
      firstValueFrom(this.client.getPlugin({ pluginId: id })),
      firstValueFrom(this.client.listPluginVersions({ pluginId: id })),
      this.loadCategories(),
    ]);
    const plugin = response.plugin;
    if (!plugin) return null;
    return PluginDevelopmentService.toPlugin(plugin, versions.versions, categories);
  }

  async getListing(id: string): Promise<PluginListing | null> {
    const response = await firstValueFrom(this.client.getPlugin({ pluginId: id }));
    return response.plugin ? PluginDevelopmentService.toListing(response.plugin) : null;
  }

  // UpdatePlugin replaces the whole listing, so every editable field is sent,
  // including the ones the author left alone.
  async updateListing(listing: PluginListing): Promise<PluginListing | null> {
    const response = await firstValueFrom(
      this.client.updatePlugin({
        pluginId: listing.id,
        description: listing.description,
        categoryIds: listing.categoryIds,
        image: listing.image,
        features: listing.features,
        documentationLinks: listing.documentationLinks,
        visibility:
          listing.visibility === 'restricted'
            ? PluginVisibility.RESTRICTED
            : PluginVisibility.PUBLIC,
        allowedOrganizationIds:
          listing.visibility === 'restricted' ? listing.allowedOrganizationIds : [],
        authorName: listing.authorName,
        authorUrl: listing.authorUrl,
        logo: dataUriBytes(listing.logo),
        logoDark: dataUriBytes(listing.logoDark),
      }),
    );
    return response.plugin ? PluginDevelopmentService.toListing(response.plugin) : null;
  }

  async listCategories(): Promise<Category[]> {
    const categories = await this.loadCategories();
    return [...categories]
      .map(([id, name]) => ({ id, name }))
      .sort((a, b) => a.name.localeCompare(b.name));
  }

  // Moves a draft, changes_requested or withdrawn version to pending and opens
  // a submission in the review queue.
  async submitVersion(versionId: string): Promise<PluginVersion | null> {
    const response = await firstValueFrom(
      this.client.submitPluginVersion({ pluginVersionId: versionId }),
    );
    return response.version ? PluginDevelopmentService.toVersion(response.version) : null;
  }

  // Pulls a pending version back, closing its open submission. It can be
  // submitted again afterwards.
  async withdrawVersion(versionId: string): Promise<PluginVersion | null> {
    const response = await firstValueFrom(
      this.client.withdrawPluginVersion({ pluginVersionId: versionId }),
    );
    return response.version ? PluginDevelopmentService.toVersion(response.version) : null;
  }

  private loadCategories(): Promise<Map<string, string>> {
    // Duplicated from the catalog on purpose (FUN-20): publishing does not
    // depend on the public storefront being reachable.
    this.categories ??= firstValueFrom(this.client.listCategories({}))
      .then(
        (response) => new Map(response.categories.map((category) => [category.id, category.name])),
      )
      .catch((err: unknown) => {
        // The promise is memoized to coalesce concurrent callers, but a failure
        // must not be: keeping a rejected promise here would fail every later
        // caller for the life of the page. Clearing the slot lets the next one
        // retry.
        this.categories = undefined;
        throw err;
      });
    return this.categories;
  }

  private static toPlugin(
    plugin: RegistryPlugin,
    versions: RegistryPluginVersion[],
    categories: Map<string, string>,
  ): AuthoredPlugin {
    // ListPluginVersions does not promise an order, and every "latest" on this
    // page depends on one, so sort here rather than trusting the server.
    //
    // Sorted on the raw timestamp, not the `pushedAt` day the UI renders:
    // pushing several builds a day is the normal case while iterating on a
    // plugin, and a truncated key would make those compare equal, dropping the
    // sort back to the server order it exists to distrust. The version string
    // breaks a tie so the result is at least stable.
    const sorted = [...versions]
      .sort(
        (a, b) => toEpochMs(b.created) - toEpochMs(a.created) || b.version.localeCompare(a.version),
      )
      .map(PluginDevelopmentService.toVersion);
    const latest = sorted[0];
    return {
      id: plugin.id,
      name: plugin.name,
      displayName: plugin.displayName,
      descriptionShort: plugin.descriptionShort,
      description: plugin.description,
      version: latest?.version ?? '',
      author: { name: plugin.authorName, url: plugin.authorUrl },
      repositoryUrl: plugin.repositoryUrl,
      logo: plugin.logo,
      logoDark: plugin.logoDark,
      tags: plugin.tags,
      categoryName: PluginDevelopmentService.categoryName(plugin.categoryIds, categories),
      // A listing with no versions has nothing to review yet, which is what a
      // draft is.
      status: latest?.status ?? 'draft',
      versions: sorted,
      listingPendingReview: plugin.pendingListing !== undefined,
    };
  }

  private static toListing(plugin: RegistryPlugin): PluginListing {
    const edits = plugin.pendingListing ?? {
      ...plugin,
      logo: plugin.listingLogo,
      logoDark: plugin.listingLogoDark,
    };
    return {
      id: plugin.id,
      pendingReview: plugin.pendingListing !== undefined,
      synced: {
        displayName: plugin.displayName,
        descriptionShort: plugin.descriptionShort,
        tags: plugin.tags,
        repositoryUrl: plugin.repositoryUrl,
        license: plugin.license,
      },
      description: edits.description,
      categoryIds: edits.categoryIds,
      image: edits.image,
      features: edits.features.map(({ id, title, body }) => ({ id, title, body })),
      documentationLinks: edits.documentationLinks.map(({ id, title, urlName, url }) => ({
        id,
        title,
        urlName,
        url,
      })),
      visibility: plugin.visibility === PluginVisibility.RESTRICTED ? 'restricted' : 'public',
      allowedOrganizationIds: plugin.allowedOrganizationIds,
      authorName: edits.authorName,
      authorUrl: edits.authorUrl,
      logo: edits.logo,
      logoDark: edits.logoDark,
      versionLogo: plugin.listingLogo ? '' : plugin.logo,
      versionLogoDark: plugin.listingLogoDark ? '' : plugin.logoDark,
    };
  }

  private static toVersion(version: RegistryPluginVersion): PluginVersion {
    return {
      id: version.id,
      version: version.version,
      pushedAt: toIsoDate(version.created),
      status: fromProtoStatus(version.status),
      image: version.image,
      reviewFeedback: version.reviewFeedback || undefined,
    };
  }

  private static categoryName(categoryIds: string[], categories: Map<string, string>): string {
    const first = categoryIds[0];
    return first ? (categories.get(first) ?? '') : '';
  }
}
