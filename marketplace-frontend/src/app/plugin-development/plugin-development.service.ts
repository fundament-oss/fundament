import { Injectable, inject } from '@angular/core';
import { firstValueFrom } from 'rxjs';
import { PUBLICATION_CLIENT } from '../../connect/tokens';
import toIsoDate from '../../connect/timestamp';
import { type SubmissionStatus, fromProtoStatus } from '../status/submission-status';
import {
  type Plugin as RegistryPlugin,
  type PluginVersion as RegistryPluginVersion,
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
  icon: string; // base name under /img/plugins/<icon>.svg
  tags: string[];
  categoryName: string;
  // A listing carries no review state of its own (FUN-20): the reviewed unit is
  // a version, so this is the newest version's status.
  status: SubmissionStatus;
  // Newest first.
  versions: PluginVersion[];
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
    this.categories ??= firstValueFrom(this.client.listCategories({})).then(
      (response) => new Map(response.categories.map((category) => [category.id, category.name])),
    );
    return this.categories;
  }

  private static toPlugin(
    plugin: RegistryPlugin,
    versions: RegistryPluginVersion[],
    categories: Map<string, string>,
  ): AuthoredPlugin {
    // ListPluginVersions does not promise an order, and every "latest" on this
    // page depends on one, so sort here rather than trusting the server.
    const sorted = [...versions]
      .map(PluginDevelopmentService.toVersion)
      .sort((a, b) => b.pushedAt.localeCompare(a.pushedAt));
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
      icon: plugin.name,
      tags: plugin.tags,
      categoryName: PluginDevelopmentService.categoryName(plugin.categoryIds, categories),
      // A listing with no versions has nothing to review yet, which is what a
      // draft is.
      status: latest?.status ?? 'draft',
      versions: sorted,
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
