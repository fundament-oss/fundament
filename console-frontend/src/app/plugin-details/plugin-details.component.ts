import { Component, inject, OnInit, signal, ChangeDetectionStrategy } from '@angular/core';
import { DomSanitizer, SafeHtml } from '@angular/platform-browser';
import { ActivatedRoute } from '@angular/router';
import { NgIcon, provideIcons } from '@ng-icons/core';
import { tablerChevronRight, tablerCheck } from '@ng-icons/tabler-icons';
import { tablerCircleXFill } from '@ng-icons/tabler-icons/fill';
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
import { ToastService } from '../toast.service';
import { PluginInstallationService } from '../plugin-installation/plugin-installation.service';

// Extended cluster type for UI state
interface ClusterWithState extends ClusterSummary {
  installed: boolean;
  running: boolean;
}

@Component({
  selector: 'app-plugin-details',
  imports: [InstallPluginModalComponent, NgIcon, LoadingIndicatorComponent],
  viewProviders: [
    provideIcons({
      tablerChevronRight,
      tablerCheck,
      tablerCircleXFill,
    }),
  ],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './plugin-details.component.html',
})
export default class PluginDetailsComponent implements OnInit {
  private titleService = inject(TitleService);

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

  showInstallModal = false;

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
          installed: installResults[i].some((item) => item.metadata.name === pluginName),
          running: cluster.status === ClusterStatus.RUNNING,
        })),
      );

      this.isLoading.set(false);
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
      .replace(/^# (.*$)/gim, '<h1 class="text-2xl font-semibold mb-3 dark:text-white">$1</h1>')
      .replace(
        /^## (.*$)/gim,
        '<h2 class="text-xl font-semibold mb-2 mt-4 dark:text-white">$1</h2>',
      )
      .replace(/^- (.*$)/gim, '<li class="ml-4">$1</li>')
      .replace(/\*\*(.*?)\*\*/g, '<strong class="font-semibold">$1</strong>')
      .replace(/\n\n/g, '</p><p class="mb-3">')
      .trim();

    html = `<p class="mb-3">${html}</p>`;

    return this.sanitizer.sanitize(1, html) || '';
  }

  openInstallModal(): void {
    this.showInstallModal = true;
  }

  closeInstallModal(): void {
    this.showInstallModal = false;
  }

  async onInstallOnCluster(clusterId: string): Promise<void> {
    const cluster = this.clusters().find((c) => c.id === clusterId);
    const plugin = this.plugin();
    if (!cluster || cluster.installed || !plugin) {
      return;
    }

    try {
      await this.pluginInstallationService.installPlugin(
        clusterId,
        plugin.name,
        this.pluginImage,
      );
      this.clusters.update((clusters) =>
        clusters.map((c) => (c.id === clusterId ? { ...c, installed: true } : c)),
      );
      this.toastService.success(`${plugin.name} installed on ${cluster.name}`);
    } catch {
      this.toastService.error(`Failed to install ${plugin.name} on ${cluster.name}`);
    }
  }

  isInstalled(clusterId: string): boolean {
    return this.clusters().some((c) => c.id === clusterId && c.installed);
  }

  hasInstalledClusters(): boolean {
    return this.clusters().some((c) => c.installed);
  }

  getInstalledClusterCount(): number {
    return this.clusters().filter((c) => c.installed).length;
  }
}
