import {
  Component,
  inject,
  DestroyRef,
  OnInit,
  signal,
  computed,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
  SecurityContext,
} from '@angular/core';
import { DomSanitizer, SafeHtml } from '@angular/platform-browser';
import { ActivatedRoute, Router } from '@angular/router';
import { create } from '@bufbuild/protobuf';
import { firstValueFrom } from 'rxjs';
import { createIdempotencyRef } from '../../connect/idempotency';
import PageNavService from '../page-nav.service';
import { TitleService } from '../title.service';
import InstallPluginModalComponent, {
  type PluginVersionOption,
  type InstallSelection,
  type RetrySelection,
} from '../install-plugin-modal/install-plugin-modal';
import { PluginIconComponent } from '../icons';
import renderMarkdown from '../markdown';
import { CLUSTER, CATALOG, INSTALL } from '../../connect/tokens';
import {
  GetPluginRequestSchema,
  ListCategoriesRequestSchema,
  ListPublishersRequestSchema,
  ListPluginVersionsRequestSchema,
} from '../../generated/catalog/v1/catalog_pb';
import { type DocumentationLink } from '../../generated/marketplace/v1/common_pb';
import {
  ListClustersRequestSchema,
  type ListClustersResponse_ClusterSummary as ClusterSummary,
} from '../../generated/v1/cluster_pb';
import {
  hasInvalidConfig,
  installPhase,
  isInstallInProgress,
  isInstallTerminating,
} from '../utils/plugin-install-status';
import { type PluginInstallationItem } from '../plugin-resources/types';
import { NotificationService } from '../notification.service';
import PluginInstallationService, {
  pluginResourceName,
  RetryAbortedError,
  RetryConfigVersionError,
} from '../plugin-installation/plugin-installation.service';
import PluginInstallWatchService, {
  type InstallWatchTarget,
} from '../plugin-installation/install-watch.service';
import injectBrowsePluginsUrl from '../plugins/browse-plugins-url';
import createInstallPoller from '../utils/install-poller';

import '@nldd/design-system/activity-indicator';
import '@nldd/design-system/banner';
import '@nldd/design-system/box';
import '@nldd/design-system/button';
import '@nldd/design-system/cell';
import '@nldd/design-system/container';
import '@nldd/design-system/icon-cell';
import '@nldd/design-system/link';
import '@nldd/design-system/list';
import '@nldd/design-system/list-item';
import '@nldd/design-system/page';
import '@nldd/design-system/rich-text';
import '@nldd/design-system/simple-section';
import '@nldd/design-system/spacer';
import '@nldd/design-system/spacer-cell';
import '@nldd/design-system/tag';
import '@nldd/design-system/text-cell';
import '@nldd/design-system/title';
import '@nldd/design-system/top-title-bar';
import { isClusterReachable } from '../utils/cluster-status';
// The detail page's own shape. catalog.v1 returns ids where organization.v1
// returned names, and splits the author into two scalars; resolving that here
// keeps the template as it was.
interface PluginDetailView {
  id: string;
  name: string;
  displayName: string;
  description: string;
  organizationName: string;
  author?: { name: string; url: string };
  categories: { id: string; name: string }[];
  tags: string[];
  documentationLinks: DocumentationLink[];
  repositoryUrl: string;
  logo: string;
  logoDark: string;
}

// Extended cluster type for UI state. `phase` is null when the plugin is not
// installed on the cluster, otherwise the PluginInstallation status phase.
interface ClusterWithState extends ClusterSummary {
  phase: string | null;
  // The version pinned on this cluster; empty when not installed.
  version: string;
  running: boolean;
  // True when a Failed phase is config-caused (ConfigValid=False): the modal
  // hides Retry for it, since retry replays the same config.
  configInvalid: boolean;
  // Why the installation is in its phase (status.message); '' when not
  // installed.
  message: string;
}

// The name to show a user (e.g. "OpenFSC"). `plugin.name` is the install
// identifier (e.g. "openfsc") — it names the PluginInstallation resource in the
// cluster — so it is never shown. Falls back to it when no display name is set.
const displayNameOf = (plugin: { name: string; displayName: string }): string =>
  plugin.displayName || plugin.name;

/** The phase to show for a cluster: null when the plugin is not installed on
 *  it. An installation the controller has not given a phase yet is still being
 *  installed, not absent: reading it as absent offers an Install that can only
 *  answer 409. */
function installPhaseOf(installation: PluginInstallationItem | undefined): string | null {
  if (!installation) return null;
  return installPhase(installation.status?.phase);
}

/** The "official" marker reads as a property of the name, not as one entry in a
 *  tag row. */
function isOfficialPlugin(plugin: { tags: string[] }): boolean {
  return plugin.tags.some((tag) => tag.toLowerCase() === 'official');
}

@Component({
  selector: 'app-plugin-details',
  imports: [InstallPluginModalComponent, PluginIconComponent],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './plugin-details.component.html',
})
export default class PluginDetailsComponent implements OnInit {
  protected pageNav = inject(PageNavService);

  private titleService = inject(TitleService);

  isOfficial = isOfficialPlugin;

  /** The parent page's name, which depends on whether it lists the whole
   *  catalog or only what is installed (see injectBrowsePluginsUrl). */
  protected readonly backText = injectBrowsePluginsUrl() ? 'Installed plugins' : 'Plugins';

  private installPoller = createInstallPoller(inject(DestroyRef), () =>
    this.refreshInstallStates(),
  );

  // Bumped by every change this page makes to a cluster's phase itself, so a
  // poll read that started before it can tell it is stale and drop it.
  private localChanges = 0;

  private sanitizer = inject(DomSanitizer);

  private route = inject(ActivatedRoute);

  private router = inject(Router);

  private catalogClient = inject(CATALOG);

  // Listings, details and versions come from install.v1 as the current
  // organization, so RESTRICTED plugins it may install appear (FUN-22).
  // Categories, publishers and presets stay on the anonymous storefront.
  private installClient = inject(INSTALL);

  private clusterClient = inject(CLUSTER);

  private notificationService = inject(NotificationService);

  private pluginInstallationService = inject(PluginInstallationService);

  private installWatch = inject(PluginInstallWatchService);

  private idempotency = createIdempotencyRef();

  pluginId = signal<string>('');

  plugin = signal<PluginDetailView | null>(null);

  // The description sits directly under the page's own title.
  renderedMarkdown = computed<SafeHtml>(() => {
    const html = renderMarkdown(this.plugin()?.description || '', 1);
    return this.sanitizer.sanitize(SecurityContext.HTML, html) || '';
  });

  clusters = signal<ClusterWithState[]>([]);

  // Published versions of this plugin, offered in the install modal.
  installVersions = signal<PluginVersionOption[]>([]);

  // True when fetching the versions failed, so the modal can show an error
  // state instead of the "no published version" notice.
  installVersionsError = signal(false);

  isLoading = signal<boolean>(true);

  errorMessage = signal<string | null>(null);

  showInstallModal = signal(false);

  async ngOnInit() {
    // Get plugin ID from route
    const id = this.route.snapshot.paramMap.get('id');
    if (!id) {
      this.errorMessage.set('Plugin ID is missing');
      this.isLoading.set(false);
      return;
    }

    this.pluginId.set(id);

    try {
      const [pluginResponse, categoriesResponse, publishersResponse, clustersResponse] =
        await Promise.all([
          firstValueFrom(
            this.installClient.getPlugin(create(GetPluginRequestSchema, { pluginId: id })),
          ),
          firstValueFrom(
            this.catalogClient.listCategories(create(ListCategoriesRequestSchema, {})),
          ),
          firstValueFrom(
            this.installClient.listPublishers(create(ListPublishersRequestSchema, {})),
          ),
          firstValueFrom(this.clusterClient.listClusters(create(ListClustersRequestSchema, {}))),
        ]);

      if (!pluginResponse.plugin) {
        this.errorMessage.set('Plugin not found');
        this.isLoading.set(false);
        return;
      }

      const detail = pluginResponse.plugin;
      const categoryNames = new Map(categoriesResponse.categories.map((c) => [c.id, c.name]));
      const publisherNames = new Map(publishersResponse.publishers.map((p) => [p.id, p.name]));

      const plugin: PluginDetailView = {
        id: detail.id,
        name: detail.name,
        displayName: detail.displayName,
        description: detail.description,
        // See plugins.component.ts: the id is a traceable stand-in, '' is not.
        organizationName: publisherNames.get(detail.organizationId) ?? detail.organizationId,
        author: detail.authorName ? { name: detail.authorName, url: detail.authorUrl } : undefined,
        categories: detail.categoryIds.map((cid) => ({
          id: cid,
          name: categoryNames.get(cid) ?? cid,
        })),
        tags: detail.tags,
        documentationLinks: detail.documentationLinks,
        repositoryUrl: detail.repositoryUrl,
        logo: detail.logo,
        logoDark: detail.logoDark,
      };

      this.plugin.set(plugin);
      this.titleService.setTitle(displayNameOf(plugin));

      // metadata.name is the RFC-1123 slug of (organizationName, pluginName) — the
      // qualified pair is the plugin's identity, so match on that, not pluginName alone.
      const resourceName = pluginResourceName(plugin.organizationName, plugin.name);

      const installResults = await Promise.all(
        clustersResponse.clusters.map((cluster) =>
          this.pluginInstallationService.listInstallations(cluster.id).catch(() => []),
        ),
      );

      this.clusters.set(
        clustersResponse.clusters.map((cluster, i) => {
          const installation = installResults[i].find(
            (item) => item.metadata.name === resourceName,
          );
          return {
            ...cluster,
            phase: installPhaseOf(installation),
            version: installation?.spec?.definitionRef?.pluginVersion ?? '',
            configInvalid: installation ? hasInvalidConfig(installation) : false,
            message: installation?.status?.message ?? '',
            running: isClusterReachable(cluster.status),
          };
        }),
      );

      this.isLoading.set(false);
      this.startInstallPollingIfNeeded();

      // The marketplace's "Install plugin" button lands here with ?install=1:
      // the visitor already chose to install, so the sheet opens straight away
      // instead of making them find the button again on this page.
      if (this.route.snapshot.queryParamMap.has('install')) {
        await this.openInstallModal();
      }
    } catch (error) {
      this.errorMessage.set(
        error instanceof Error
          ? `Failed to load plugin details: ${error.message}`
          : 'Failed to load plugin details',
      );
      this.isLoading.set(false);
    }
  }

  async openInstallModal(): Promise<void> {
    const plugin = this.plugin();
    this.installVersionsError.set(false);
    if (!plugin) {
      this.installVersions.set([]);
      this.showInstallModal.set(true);
      return;
    }
    try {
      this.installVersions.set(await this.fetchPluginVersions(plugin.id));
    } catch {
      this.installVersions.set([]);
      this.installVersionsError.set(true);
    }
    this.showInstallModal.set(true);
  }

  // Fetches the plugin's published versions (latest first) for the install
  // modal's version picker. Throws on failure so the caller can tell a fetch
  // error apart from a plugin that simply has nothing published yet.
  private async fetchPluginVersions(pluginId: string): Promise<PluginVersionOption[]> {
    const resp = await firstValueFrom(
      this.installClient.listPluginVersions(create(ListPluginVersionsRequestSchema, { pluginId })),
    );
    return resp.versions.map((v) => ({ version: v.version, hash: v.definitionHash }));
  }

  closeInstallModal(): void {
    this.showInstallModal.set(false);
    this.installVersions.set([]);
    this.installVersionsError.set(false);

    // Drop the hand-off flag once the sheet is dismissed, so a reload or a
    // shared link shows the page rather than opening the sheet again.
    if (this.route.snapshot.queryParamMap.has('install')) {
      this.router
        .navigate([], {
          relativeTo: this.route,
          queryParams: { install: null },
          queryParamsHandling: 'merge',
          replaceUrl: true,
        })
        .catch(() => {});
    }
  }

  private clusterName(clusterId: string): string {
    return this.clusters().find((c) => c.id === clusterId)?.name ?? clusterId;
  }

  /** The plugin on a cluster, as the install watcher names it in a
   *  notification. */
  private watchTarget(plugin: PluginDetailView, clusterId: string): InstallWatchTarget {
    return {
      clusterId,
      clusterName: this.clusterName(clusterId),
      organizationName: plugin.organizationName,
      pluginName: plugin.name,
      pluginLabel: displayNameOf(plugin),
    };
  }

  private setPhase(clusterId: string, phase: string | null): void {
    this.localChanges += 1;
    this.clusters.update((clusters) =>
      // The message belongs to the phase the poll last read; the next poll fills it in.
      clusters.map((c) => (c.id === clusterId ? { ...c, phase, message: '' } : c)),
    );
  }

  /** Starts, re-paces or stops the install poll to suit the clusters shown.
   *  Also while one is Degraded or Failed, at a slower pace: it can recover, or
   *  get a new reason, without anyone acting on it. */
  private startInstallPollingIfNeeded(): void {
    this.installPoller.pace(
      this.clusters().flatMap((c) =>
        c.phase === null ? [] : [{ phase: c.phase, configInvalid: c.configInvalid }],
      ),
    );
  }

  // Poll installation status and surface transitions (installed / failed / removed).
  private async refreshInstallStates(): Promise<void> {
    const plugin = this.plugin();
    if (!plugin) return;

    const resourceName = pluginResourceName(plugin.organizationName, plugin.name);
    const clusters = this.clusters();
    const changes = this.localChanges;
    const readToken = this.installWatch.readToken();
    let results: (PluginInstallationItem[] | null)[];
    try {
      results = await Promise.all(
        clusters.map((c) =>
          this.pluginInstallationService
            .listInstallations(c.id)
            .catch((): PluginInstallationItem[] | null => null),
        ),
      );
    } catch {
      return;
    }
    // An install, uninstall or retry since the read started: what it read is
    // from before, and would put the row back to the phase it had. The handler
    // re-paced the poll, so the next read is not far off.
    if (changes !== this.localChanges) return;

    const next = clusters.map((c, i) => {
      const items = results[i];
      // Couldn't read this cluster — keep its current state rather than treating
      // an unreadable cluster as "plugin removed".
      if (items === null) return c;
      const install = items.find((item) => item.metadata.name === resourceName);
      const phase = installPhaseOf(install);
      // A 'Pending' row that has vanished is an optimistic install (or in-flight
      // retry) the backend has not listed yet — keep showing it as pending.
      const resolved = phase === null && c.phase === 'Pending' ? 'Pending' : phase;
      return {
        ...c,
        phase: resolved,
        configInvalid: install ? hasInvalidConfig(install) : false,
        message: install?.status?.message ?? '',
        running: isClusterReachable(c.status),
      };
    });

    next.forEach((n, i) => {
      const prevPhase = clusters[i].phase;
      if (prevPhase === null || n.phase === prevPhase) return;
      this.installWatch.observe(
        this.watchTarget(plugin, n.id),
        prevPhase,
        n.phase,
        readToken,
        n.message,
      );
    });

    this.clusters.set(next);
    this.startInstallPollingIfNeeded();
  }

  async onInstallOnClusters(selection: InstallSelection): Promise<void> {
    const plugin = this.plugin();
    if (!plugin) return;

    const targets = selection.clusterIds.filter(
      (id) => this.clusters().find((c) => c.id === id)?.phase === null,
    );
    if (targets.length === 0) return;

    targets.forEach((id) => this.setPhase(id, 'Pending'));

    // Each install is watched as soon as its cluster accepted it: a cluster
    // that is slow to answer must not hold up following the others.
    const results = await Promise.allSettled(
      targets.map((id) =>
        this.pluginInstallationService
          .installPlugin(
            id,
            plugin.organizationName,
            plugin.name,
            selection.version,
            selection.hash,
            selection.config,
          )
          .then(() => this.installWatch.watch(this.watchTarget(plugin, id), 'install')),
      ),
    );

    const failed = targets.filter((_, i) => results[i].status === 'rejected');
    if (failed.length > 0) {
      failed.forEach((id) => this.setPhase(id, null));
      this.notificationService.error(
        `Failed to install ${displayNameOf(plugin)} on ${failed.map((id) => this.clusterName(id)).join(', ')}`,
      );
    }

    this.startInstallPollingIfNeeded();
  }

  async onUninstallFromCluster(clusterId: string): Promise<void> {
    const plugin = this.plugin();
    if (!plugin) return;

    try {
      await this.pluginInstallationService.uninstallPlugin(
        clusterId,
        pluginResourceName(plugin.organizationName, plugin.name),
      );
      this.setPhase(clusterId, 'Terminating');
      this.installWatch.watch(this.watchTarget(plugin, clusterId), 'uninstall');
      this.startInstallPollingIfNeeded();
    } catch {
      this.notificationService.error(
        `Failed to remove ${displayNameOf(plugin)} from ${this.clusterName(clusterId)}`,
      );
    }
  }

  async onRetryInstall(retry: RetrySelection): Promise<void> {
    const plugin = this.plugin();
    if (!plugin) return;

    const clusterId = retry.clusterId;
    const previous = this.clusters().find((c) => c.id === clusterId)?.phase ?? null;
    this.setPhase(clusterId, 'Pending');

    try {
      await this.installWatch.followRetry(this.watchTarget(plugin, clusterId), () =>
        this.pluginInstallationService.retryInstall(
          clusterId,
          plugin.organizationName,
          plugin.name,
          retry.version,
          retry.hash,
        ),
      );
      this.startInstallPollingIfNeeded();
    } catch (err) {
      // An aborted retry stopped before anything was deleted — roll the row
      // back to the phase it had.
      if (err instanceof RetryAbortedError) this.setPhase(clusterId, previous);
      this.notificationService.error(
        err instanceof RetryConfigVersionError
          ? `Can't retry ${displayNameOf(plugin)} at a different version with its saved configuration — uninstall and reinstall it instead`
          : `Failed to install ${displayNameOf(plugin)} on ${this.clusterName(clusterId)}`,
      );
    }
  }

  /** Whether the header offers Manage rather than Install: true for an
   *  installation in any state but being removed. One still coming up, or
   *  Degraded or Failed, is one to manage, retry or uninstall, not one to
   *  install again. One being removed is on its way out. */
  hasInstalledClusters(): boolean {
    return this.clusters().some((c) => c.phase !== null && !isInstallTerminating(c.phase));
  }

  /** The clusters the plugin is installed on, for "Installed on N clusters":
   *  only installations that have settled, Running or not. One still on its
   *  way up or down shows as Installing… or Removing… in the install modal,
   *  and must not read as installed here meanwhile. */
  getInstalledClusterCount(): number {
    return this.clusters().filter((c) => c.phase !== null && !isInstallInProgress(c.phase)).length;
  }
}
