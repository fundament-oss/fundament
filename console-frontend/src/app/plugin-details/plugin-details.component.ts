import {
  Component,
  inject,
  OnInit,
  OnDestroy,
  signal,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
} from '@angular/core';
import { DomSanitizer, SafeHtml } from '@angular/platform-browser';
import { ActivatedRoute } from '@angular/router';
import { create } from '@bufbuild/protobuf';
import { firstValueFrom } from 'rxjs';
import { createIdempotencyRef } from '../../connect/idempotency';
import { TitleService } from '../title.service';
import InstallPluginModalComponent from '../install-plugin-modal/install-plugin-modal';
import { LoadingIndicatorComponent } from '../icons';
import { PLUGIN, CLUSTER } from '../../connect/tokens';
import {
  GetPluginDetailRequestSchema,
  ListPluginsRequestSchema,
  type PluginDetail,
} from '../../generated/v1/plugin_pb';
import {
  ListClustersRequestSchema,
  type ListClustersResponse_ClusterSummary as ClusterSummary,
} from '../../generated/v1/cluster_pb';
import { ClusterStatus } from '../../generated/v1/common_pb';
import { isInstallInProgress, isInstallRunning } from '../utils/plugin-install-status';
import { type PluginInstallationItem } from '../plugin-resources/types';
import { ToastService } from '../toast.service';
import PluginInstallationService from '../plugin-installation/plugin-installation.service';

// Extended cluster type for UI state. `phase` is null when the plugin is not
// installed on the cluster, otherwise the PluginInstallation status phase.
interface ClusterWithState extends ClusterSummary {
  phase: string | null;
  running: boolean;
}

@Component({
  selector: 'app-plugin-details',
  imports: [InstallPluginModalComponent, LoadingIndicatorComponent],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './plugin-details.component.html',
})
export default class PluginDetailsComponent implements OnInit, OnDestroy {
  private titleService = inject(TitleService);

  private installPollingTimer: ReturnType<typeof setInterval> | null = null;

  private sanitizer = inject(DomSanitizer);

  private route = inject(ActivatedRoute);

  private pluginClient = inject(PLUGIN);

  private clusterClient = inject(CLUSTER);

  private toastService = inject(ToastService);

  private pluginInstallationService = inject(PluginInstallationService);

  private idempotency = createIdempotencyRef();

  private pluginImage = '';

  pluginId = signal<string>('');

  plugin = signal<PluginDetail | null>(null);

  clusters = signal<ClusterWithState[]>([]);

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
      const [pluginResponse, clustersResponse, pluginsResponse] = await Promise.all([
        firstValueFrom(
          this.pluginClient.getPluginDetail(create(GetPluginDetailRequestSchema, { pluginId: id })),
        ),
        firstValueFrom(this.clusterClient.listClusters(create(ListClustersRequestSchema, {}))),
        firstValueFrom(this.pluginClient.listPlugins(create(ListPluginsRequestSchema, {}))),
      ]);

      if (!pluginResponse.plugin) {
        this.errorMessage.set('Plugin not found');
        this.isLoading.set(false);
        return;
      }

      this.plugin.set(pluginResponse.plugin);
      this.titleService.setTitle(`${pluginResponse.plugin.name} — Plugins`);

      const pluginName = pluginResponse.plugin.name;
      this.pluginImage = pluginsResponse.plugins.find((p) => p.id === id)?.image ?? '';

      const installResults = await Promise.all(
        clustersResponse.clusters.map((cluster) =>
          this.pluginInstallationService.listInstallations(cluster.id).catch(() => []),
        ),
      );

      this.clusters.set(
        clustersResponse.clusters.map((cluster, i) => ({
          ...cluster,
          phase:
            installResults[i].find((item) => item.metadata.name === pluginName)?.status?.phase ??
            null,
          running: cluster.status === ClusterStatus.RUNNING,
        })),
      );

      this.isLoading.set(false);
      this.startInstallPollingIfNeeded();
    } catch (error) {
      this.errorMessage.set(
        error instanceof Error
          ? `Failed to load plugin details: ${error.message}`
          : 'Failed to load plugin details',
      );
      this.isLoading.set(false);
    }
  }

  getRenderedMarkdown(): SafeHtml {
    const description = this.plugin()?.description || '';

    // Simple markdown to HTML conversion
    let html = description
      .replace(/^# (.*$)/gim, '<h1 class="text-3xl font-semibold mb-3">$1</h1>')
      .replace(/^## (.*$)/gim, '<h2 class="text-xl font-semibold mb-2 mt-4">$1</h2>')
      .replace(/^- (.*$)/gim, '<li class="ml-4">$1</li>')
      .replace(/\*\*(.*?)\*\*/g, '<strong class="font-semibold">$1</strong>')
      .replace(/\n\n/g, '</p><p class="mb-3">')
      .trim();

    html = `<p class="mb-3">${html}</p>`;

    return this.sanitizer.sanitize(1, html) || '';
  }

  openInstallModal(): void {
    this.showInstallModal.set(true);
  }

  closeInstallModal(): void {
    this.showInstallModal.set(false);
  }

  ngOnDestroy() {
    this.stopInstallPolling();
  }

  private clusterName(clusterId: string): string {
    return this.clusters().find((c) => c.id === clusterId)?.name ?? clusterId;
  }

  private setPhase(clusterId: string, phase: string | null): void {
    this.clusters.update((clusters) =>
      clusters.map((c) => (c.id === clusterId ? { ...c, phase } : c)),
    );
  }

  private startInstallPollingIfNeeded(): void {
    if (this.installPollingTimer) return;
    if (this.clusters().some((c) => c.phase !== null && isInstallInProgress(c.phase))) {
      this.installPollingTimer = setInterval(() => this.refreshInstallStates(), 5000);
    }
  }

  private stopInstallPolling(): void {
    if (this.installPollingTimer) {
      clearInterval(this.installPollingTimer);
      this.installPollingTimer = null;
    }
  }

  // Poll installation status and surface transitions (installed / failed / removed).
  private async refreshInstallStates(): Promise<void> {
    const plugin = this.plugin();
    if (!plugin) return;

    const clusters = this.clusters();
    let results: PluginInstallationItem[][];
    try {
      results = await Promise.all(
        clusters.map((c) => this.pluginInstallationService.listInstallations(c.id).catch(() => [])),
      );
    } catch {
      return;
    }

    const next = clusters.map((c, i) => ({
      ...c,
      phase: results[i].find((item) => item.metadata.name === plugin.name)?.status?.phase ?? null,
      running: c.status === ClusterStatus.RUNNING,
    }));

    next.forEach((n, i) => {
      const prevPhase = clusters[i].phase;
      if (prevPhase === null) return;
      if (!isInstallRunning(prevPhase) && n.phase === 'Running') {
        this.toastService.success(`${plugin.name} installed on ${n.name}`);
      } else if (prevPhase !== 'Failed' && n.phase === 'Failed') {
        this.toastService.error(`Failed to install ${plugin.name} on ${n.name}`);
      } else if (n.phase === null) {
        this.toastService.success(`${plugin.name} removed from ${n.name}`);
      }
    });

    this.clusters.set(next);

    if (!next.some((c) => c.phase !== null && isInstallInProgress(c.phase))) {
      this.stopInstallPolling();
    }
  }

  async onInstallOnClusters(clusterIds: string[]): Promise<void> {
    const plugin = this.plugin();
    if (!plugin) return;

    const targets = clusterIds.filter(
      (id) => this.clusters().find((c) => c.id === id)?.phase === null,
    );
    if (targets.length === 0) return;

    targets.forEach((id) => this.setPhase(id, 'Pending'));

    const results = await Promise.allSettled(
      targets.map((id) =>
        this.pluginInstallationService.installPlugin(id, plugin.name, this.pluginImage),
      ),
    );

    const failed = targets.filter((_, i) => results[i].status === 'rejected');
    if (failed.length > 0) {
      failed.forEach((id) => this.setPhase(id, null));
      this.toastService.error(
        `Failed to install ${plugin.name} on ${failed.map((id) => this.clusterName(id)).join(', ')}`,
      );
    }

    this.startInstallPollingIfNeeded();
  }

  async onUninstallFromCluster(clusterId: string): Promise<void> {
    const plugin = this.plugin();
    if (!plugin) return;

    try {
      await this.pluginInstallationService.uninstallPlugin(clusterId, plugin.name);
      this.setPhase(clusterId, 'Terminating');
      this.startInstallPollingIfNeeded();
    } catch {
      this.toastService.error(
        `Failed to remove ${plugin.name} from ${this.clusterName(clusterId)}`,
      );
    }
  }

  async onRetryInstall(clusterId: string): Promise<void> {
    const plugin = this.plugin();
    if (!plugin) return;

    this.setPhase(clusterId, 'Pending');
    try {
      await this.pluginInstallationService.uninstallPlugin(clusterId, plugin.name).catch(() => {});
      await this.waitForUninstall(clusterId, plugin.name);
      await this.pluginInstallationService.installPlugin(clusterId, plugin.name, this.pluginImage);
      this.startInstallPollingIfNeeded();
    } catch {
      this.toastService.error(`Failed to install ${plugin.name} on ${this.clusterName(clusterId)}`);
    }
  }

  private async waitForUninstall(
    clusterId: string,
    pluginName: string,
    attempts = 10,
  ): Promise<void> {
    if (attempts <= 0) return;
    const items = await this.pluginInstallationService.listInstallations(clusterId).catch(() => []);
    if (!items.some((item) => item.metadata.name === pluginName)) return;
    await new Promise((resolve) => {
      setTimeout(resolve, 1000);
    });
    await this.waitForUninstall(clusterId, pluginName, attempts - 1);
  }

  hasInstalledClusters(): boolean {
    return this.clusters().some((c) => isInstallRunning(c.phase ?? ''));
  }

  getInstalledClusterCount(): number {
    return this.clusters().filter((c) => isInstallRunning(c.phase ?? '')).length;
  }
}
