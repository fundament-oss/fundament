import {
  Component,
  inject,
  signal,
  OnInit,
  OnDestroy,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
} from '@angular/core';
import { RouterLink } from '@angular/router';
import { create } from '@bufbuild/protobuf';
import { firstValueFrom } from 'rxjs';
import { TitleService } from '../title.service';
import InstallPluginModalComponent from '../install-plugin-modal/install-plugin-modal';
import { LoadingIndicatorComponent } from '../icons';
import { OrganizationDataService } from '../organization-data.service';
import { PLUGIN, CLUSTER } from '../../connect/tokens';
import {
  ListPluginsRequestSchema,
  ListPresetsRequestSchema,
  type Category,
  type Preset,
  type PluginSummary,
} from '../../generated/v1/plugin_pb';
import { type ListClustersResponse_ClusterSummary as ClusterSummary } from '../../generated/v1/cluster_pb';
import { ClusterStatus } from '../../generated/v1/common_pb';
import { isTransitionalStatus } from '../utils/cluster-status';
import { isInstallInProgress, isInstallRunning } from '../utils/plugin-install-status';
import { ToastService } from '../toast.service';
import PluginInstallationService from '../plugin-installation/plugin-installation.service';

const getPluginIconName = (pluginName: string): string =>
  pluginName.toLowerCase().replace(/[^a-z]+/g, '-');

// Extended plugin type with presets array (computed from backend data)
interface PluginWithPresets extends Pick<
  PluginSummary,
  'id' | 'name' | 'descriptionShort' | 'description' | 'categories' | 'tags' | 'image'
> {
  presets?: string[]; // Array of preset IDs this plugin belongs to
}

// Cluster row data passed to the install modal
interface ClusterModalRow {
  id: string;
  name: string;
  // null when the plugin is not installed on this cluster; otherwise the
  // PluginInstallation status phase.
  phase: string | null;
  running: boolean;
}

// Extended install type with cluster ID and live status phase
interface InstallWithCluster {
  clusterId: string;
  pluginName: string;
  phase: string;
  ready: boolean;
}

// Extended category type with count for filtering
interface CategoryWithCount extends Pick<Category, 'id' | 'name'> {
  count: number;
}

// Extended preset type with count for filtering
interface PresetWithCount extends Pick<Preset, 'id' | 'name' | 'description'> {
  count: number;
}

@Component({
  selector: 'app-plugins',
  imports: [RouterLink, InstallPluginModalComponent, LoadingIndicatorComponent],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './plugins.component.html',
})
export default class PluginsComponent implements OnInit, OnDestroy {
  private titleService = inject(TitleService);

  private pluginClient = inject(PLUGIN);

  private clusterClient = inject(CLUSTER);

  private pollingTimer: ReturnType<typeof setInterval> | null = null;

  private installPollingTimer: ReturnType<typeof setInterval> | null = null;

  private organizationDataService = inject(OrganizationDataService);

  private toastService = inject(ToastService);

  private pluginInstallationService = inject(PluginInstallationService);

  selectedCategory = 'all';

  selectedPreset = 'all';

  showInstallModal = signal(false);

  selectedPlugin: PluginWithPresets | null = null;

  isLoading = signal(true);

  errorMessage = signal<string | null>(null);

  clusters = signal<ClusterSummary[]>([]);

  installs = signal<InstallWithCluster[]>([]);

  get presets(): PresetWithCount[] {
    const presetCounts = new Map<string, number>();

    // Count plugins per preset based on current category filter
    this.plugins.forEach((plugin) => {
      const matchesCategory =
        this.selectedCategory === 'all' ||
        plugin.categories.some((cat) => cat.id === this.selectedCategory);

      if (matchesCategory && plugin.presets) {
        plugin.presets.forEach((presetId: string) => {
          presetCounts.set(presetId, (presetCounts.get(presetId) || 0) + 1);
        });
      }
    });

    // Count all plugins for 'all' preset
    const allCount = this.plugins.filter(
      (plugin) =>
        this.selectedCategory === 'all' ||
        plugin.categories.some((cat) => cat.id === this.selectedCategory),
    ).length;

    // Build presets list from backend presets
    const presets: PresetWithCount[] = [
      { id: 'all', name: 'All presets', description: 'Show all plugins', count: allCount },
    ];

    this.backendPresets.forEach((backendPreset) => {
      presets.push({
        id: backendPreset.id,
        name: backendPreset.name,
        description: backendPreset.description,
        count: presetCounts.get(backendPreset.id) || 0,
      });
    });

    return presets;
  }

  plugins: PluginWithPresets[] = [];

  backendPresets: Preset[] = [];

  async ngOnInit() {
    try {
      // Fetch plugins and presets in parallel; use pre-fetched cluster data from service
      const [pluginsResponse, presetsResponse] = await Promise.all([
        firstValueFrom(this.pluginClient.listPlugins(create(ListPluginsRequestSchema, {}))),
        firstValueFrom(this.pluginClient.listPresets(create(ListPresetsRequestSchema, {}))),
      ]);

      // Store backend presets
      this.backendPresets = presetsResponse.presets;

      // Map backend plugins to frontend format and assign presets
      this.plugins = pluginsResponse.plugins.map((backendPlugin) => {
        const assignedPresets: string[] = [];

        // Check which presets include this plugin
        this.backendPresets.forEach((preset) => {
          if (preset.pluginIds.includes(backendPlugin.id)) {
            assignedPresets.push(preset.id);
          }
        });

        return {
          id: backendPlugin.id,
          name: backendPlugin.name,
          description: backendPlugin.description,
          descriptionShort: backendPlugin.descriptionShort,
          categories: backendPlugin.categories,
          tags: backendPlugin.tags,
          image: backendPlugin.image,
          presets: assignedPresets,
        };
      });

      // Use pre-fetched cluster summaries instead of making a duplicate ListClusters call
      this.clusters.set(this.organizationDataService.clusterSummaries());

      this.installs.set(await this.fetchInstalls());

      this.isLoading.set(false);

      // Poll for cluster readiness so the install modal reflects status changes
      // (e.g. a provisioning cluster becoming RUNNING) without a page refresh.
      if (this.clusters().some((c) => isTransitionalStatus(c.status))) {
        this.pollingTimer = setInterval(() => this.refreshClusters(), 5000);
      }

      this.startInstallPollingIfNeeded();
    } catch (error) {
      this.errorMessage.set(
        error instanceof Error ? `Failed to load data: ${error.message}` : 'Failed to load data',
      );
      this.isLoading.set(false);
    }
  }

  ngOnDestroy() {
    this.stopPolling();
    this.stopInstallPolling();
  }

  private async refreshClusters() {
    try {
      const response = await firstValueFrom(this.clusterClient.listClusters({}));
      this.clusters.set(response.clusters);

      const needsPolling = response.clusters.some((c) => isTransitionalStatus(c.status));
      if (needsPolling && !this.pollingTimer) {
        this.pollingTimer = setInterval(() => this.refreshClusters(), 5000);
      } else if (!needsPolling) {
        this.stopPolling();
      }
    } catch {
      // Ignore errors from background polling.
    }
  }

  private stopPolling() {
    if (this.pollingTimer) {
      clearInterval(this.pollingTimer);
      this.pollingTimer = null;
    }
  }

  // Fetch the current installations per cluster with their live phase. A
  // cluster's `installs` is null when its listInstallations call failed, so the
  // caller can tell "this cluster has no installs" apart from "we couldn't read
  // this cluster" and avoid mistaking a failed read for an uninstall.
  private async fetchInstallsByCluster(): Promise<
    { clusterId: string; installs: InstallWithCluster[] | null }[]
  > {
    const clusters = this.clusters();
    const results = await Promise.all(
      clusters.map((cluster) =>
        this.pluginInstallationService
          .listInstallations(cluster.id)
          .then((items) =>
            items.map((item) => ({
              clusterId: cluster.id,
              pluginName: item.metadata.name,
              phase: item.status?.phase ?? 'Pending',
              ready: item.status?.ready ?? false,
            })),
          )
          .catch((): InstallWithCluster[] | null => null),
      ),
    );
    return clusters.map((cluster, i) => ({ clusterId: cluster.id, installs: results[i] }));
  }

  // Flattened view used for the initial load, where there is no previous state
  // to reconcile against (a failed cluster simply contributes no installs).
  private async fetchInstalls(): Promise<InstallWithCluster[]> {
    const byCluster = await this.fetchInstallsByCluster();
    return byCluster.flatMap((cluster) => cluster.installs ?? []);
  }

  private startInstallPollingIfNeeded() {
    if (this.installPollingTimer) return;
    if (this.installs().some((install) => isInstallInProgress(install.phase))) {
      this.installPollingTimer = setInterval(() => this.refreshInstalls(), 5000);
    }
  }

  private stopInstallPolling() {
    if (this.installPollingTimer) {
      clearInterval(this.installPollingTimer);
      this.installPollingTimer = null;
    }
  }

  // Poll installation status and surface transitions (installed / failed / removed).
  private async refreshInstalls() {
    let byCluster: { clusterId: string; installs: InstallWithCluster[] | null }[];
    try {
      byCluster = await this.fetchInstallsByCluster();
    } catch {
      return; // Ignore errors from background polling.
    }

    const previous = this.installs();

    // Only reconcile clusters we could actually read this cycle; a failed read
    // must not be mistaken for an uninstall.
    const readClusterIds = new Set(
      byCluster.filter((c) => c.installs !== null).map((c) => c.clusterId),
    );
    const fresh = byCluster.flatMap((c) => c.installs ?? []);

    // Detect phase transitions for in-flight installs.
    fresh.forEach((next) => {
      const prev = previous.find(
        (p) => p.clusterId === next.clusterId && p.pluginName === next.pluginName,
      );
      if (!prev) return;
      if (!isInstallRunning(prev.phase) && isInstallRunning(next.phase)) {
        this.toastService.success(
          `Plugin ${next.pluginName} installed on cluster ${this.clusterName(next.clusterId)}`,
        );
      } else if (prev.phase !== 'Failed' && next.phase === 'Failed') {
        this.toastService.error(
          `Failed to install plugin ${next.pluginName} on cluster ${this.clusterName(next.clusterId)}`,
        );
      }
    });

    // Detect installs that vanished from a cluster we successfully read. A
    // 'Pending' entry is an optimistic install (or an in-flight retry) the
    // backend has not listed yet — keep it and stay quiet; anything else that
    // disappeared is a completed uninstall.
    const preserved: InstallWithCluster[] = [];
    previous.forEach((prev) => {
      if (!readClusterIds.has(prev.clusterId)) return;
      const stillThere = fresh.some(
        (f) => f.clusterId === prev.clusterId && f.pluginName === prev.pluginName,
      );
      if (stillThere) return;
      if (prev.phase === 'Pending') {
        preserved.push(prev);
        return;
      }
      this.toastService.success(
        `Plugin ${prev.pluginName} removed from ${this.clusterName(prev.clusterId)}`,
      );
    });

    const merged = [
      // Keep rows for clusters we couldn't read this cycle.
      ...previous.filter((p) => !readClusterIds.has(p.clusterId)),
      ...fresh,
      ...preserved,
    ];
    this.installs.set(merged);

    if (!merged.some((install) => isInstallInProgress(install.phase))) {
      this.stopInstallPolling();
    }
  }

  get categories(): CategoryWithCount[] {
    const categoryMap = new Map<string, { name: string; count: number }>();

    // Count plugins per category based on current preset filter
    this.plugins.forEach((plugin) => {
      const matchesPreset =
        this.selectedPreset === 'all' ||
        (plugin.presets && plugin.presets.includes(this.selectedPreset));

      if (matchesPreset) {
        // A plugin can have multiple categories
        plugin.categories.forEach((category) => {
          const existing = categoryMap.get(category.id);
          if (existing) {
            existing.count += 1;
          } else {
            categoryMap.set(category.id, { name: category.name, count: 1 });
          }
        });
      }
    });

    // Count all plugins for 'all' category
    const allCount = this.plugins.filter(
      (plugin) =>
        this.selectedPreset === 'all' ||
        (plugin.presets && plugin.presets.includes(this.selectedPreset)),
    ).length;

    // Create categories array with dynamic counts
    const categories: CategoryWithCount[] = [
      { id: 'all', name: 'All categories', count: allCount },
    ];

    // Add categories from the map
    categoryMap.forEach((value, categoryId) => {
      categories.push({
        id: categoryId,
        name: value.name,
        count: value.count,
      });
    });

    return categories;
  }

  constructor() {
    this.titleService.setTitle('Plugins');
  }

  get filteredPlugins(): PluginWithPresets[] {
    return this.plugins.filter((plugin) => {
      // Filter by preset
      const matchesPreset =
        this.selectedPreset === 'all' ||
        (plugin.presets && plugin.presets.includes(this.selectedPreset));

      // Filter by category (plugin can be in multiple categories)
      const matchesCategory =
        this.selectedCategory === 'all' ||
        plugin.categories.some((cat) => cat.id === this.selectedCategory);

      return matchesPreset && matchesCategory;
    });
  }

  selectCategory(categoryId: string) {
    this.selectedCategory = categoryId;
  }

  selectPreset(presetId: string) {
    this.selectedPreset = presetId;
  }

  getSelectedCategoryName(): string {
    const category = this.categories.find((c) => c.id === this.selectedCategory);
    return category?.name || '';
  }

  // Get clusters with install state for the selected plugin
  get clustersForModal(): ClusterModalRow[] {
    const plugin = this.selectedPlugin;
    if (!plugin) {
      return [];
    }

    return this.clusters().map((cluster) => {
      const install = this.installs().find(
        (i) => i.clusterId === cluster.id && i.pluginName === plugin.name,
      );
      return {
        id: cluster.id,
        name: cluster.name,
        phase: install?.phase ?? null,
        running: cluster.status === ClusterStatus.RUNNING,
      };
    });
  }

  // True when the plugin is installed (in any phase) on at least one cluster.
  isPluginInstalledAnywhere(pluginName: string): boolean {
    return this.installs().some((install) => install.pluginName === pluginName);
  }

  // Number of clusters where the plugin is up and running.
  runningInstallCount(pluginName: string): number {
    return this.installs().filter(
      (install) => install.pluginName === pluginName && isInstallRunning(install.phase),
    ).length;
  }

  get clusterCount(): number {
    return this.clusters().length;
  }

  onInstallPlugin(plugin: PluginWithPresets) {
    this.selectedPlugin = plugin;
    this.showInstallModal.set(true);
  }

  closeInstallModal(): void {
    this.showInstallModal.set(false);
    this.selectedPlugin = null;
  }

  private clusterName(clusterId: string): string {
    return this.clusters().find((c) => c.id === clusterId)?.name ?? clusterId;
  }

  private setInstallPhase(clusterId: string, pluginName: string, phase: string): void {
    this.installs.update((current) =>
      current.map((install) =>
        install.clusterId === clusterId && install.pluginName === pluginName
          ? { ...install, phase, ready: phase === 'Running' }
          : install,
      ),
    );
  }

  async onInstallOnClusters(clusterIds: string[]): Promise<void> {
    const plugin = this.selectedPlugin;
    if (!plugin) return;

    const targets = clusterIds.filter(
      (id) => !this.installs().some((i) => i.clusterId === id && i.pluginName === plugin.name),
    );
    if (targets.length === 0) return;

    // Optimistically mark each target as installing so the rows update at once.
    this.installs.update((current) => [
      ...current,
      ...targets.map((clusterId) => ({
        clusterId,
        pluginName: plugin.name,
        phase: 'Pending',
        ready: false,
      })),
    ]);

    const results = await Promise.allSettled(
      targets.map((clusterId) =>
        this.pluginInstallationService.installPlugin(clusterId, plugin.name, plugin.image),
      ),
    );

    // Roll back optimistic entries whose install request failed outright.
    const failed = targets.filter((_, i) => results[i].status === 'rejected');
    if (failed.length > 0) {
      this.installs.update((current) =>
        current.filter(
          (install) => !(failed.includes(install.clusterId) && install.pluginName === plugin.name),
        ),
      );
      const names = failed.map((id) => this.clusterName(id)).join(', ');
      this.toastService.error(`Failed to install ${plugin.name} on ${names}`);
    }

    this.startInstallPollingIfNeeded();
  }

  async onUninstallFromCluster(clusterId: string): Promise<void> {
    const plugin = this.selectedPlugin;
    if (!plugin) return;

    try {
      await this.pluginInstallationService.uninstallPlugin(clusterId, plugin.name);
      // Optimistically mark as terminating; the poll removes it once gone.
      this.setInstallPhase(clusterId, plugin.name, 'Terminating');
      this.startInstallPollingIfNeeded();
    } catch {
      this.toastService.error(
        `Failed to remove ${plugin.name} from ${this.clusterName(clusterId)}`,
      );
    }
  }

  async onRetryInstall(clusterId: string): Promise<void> {
    const plugin = this.selectedPlugin;
    if (!plugin) return;

    this.setInstallPhase(clusterId, plugin.name, 'Pending');
    try {
      // The CRD from the failed install still exists, so remove it and wait for
      // it to be gone before re-creating (a plain re-POST would 409).
      await this.pluginInstallationService.uninstallPlugin(clusterId, plugin.name).catch(() => {});
      await this.waitForUninstall(clusterId, plugin.name);
      await this.pluginInstallationService.installPlugin(clusterId, plugin.name, plugin.image);
      this.startInstallPollingIfNeeded();
    } catch {
      this.toastService.error(`Failed to install ${plugin.name} on ${this.clusterName(clusterId)}`);
    }
  }

  private async waitForUninstall(
    clusterId: string,
    pluginName: string,
    // Wait up to ~30s for finalizers to clear the old CRD before re-creating it;
    // re-POSTing while it is still terminating would 409.
    attempts = 30,
  ): Promise<void> {
    if (attempts <= 0) return;
    const items = await this.pluginInstallationService.listInstallations(clusterId).catch(() => []);
    if (!items.some((item) => item.metadata.name === pluginName)) return;
    await new Promise((resolve) => {
      setTimeout(resolve, 1000);
    });
    await this.waitForUninstall(clusterId, pluginName, attempts - 1);
  }

  getPluginIconName = getPluginIconName;
}
