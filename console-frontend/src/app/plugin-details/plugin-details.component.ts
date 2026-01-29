import { Component, inject, OnInit, signal } from '@angular/core';
import { CommonModule } from '@angular/common';
import { DomSanitizer, SafeHtml } from '@angular/platform-browser';
import { ActivatedRoute, RouterLink } from '@angular/router';
import { TitleService } from '../title.service';
import { InstallPluginModalComponent } from '../install-plugin-modal/install-plugin-modal';
import { NgIcon, provideIcons } from '@ng-icons/core';
import { tablerChevronRight, tablerCheck } from '@ng-icons/tabler-icons';
import { tablerCircleXFill } from '@ng-icons/tabler-icons/fill';
import { LoadingIndicatorComponent } from '../icons';
import { PLUGIN, CLUSTER } from '../../connect/tokens';
import { create } from '@bufbuild/protobuf';
import { GetPluginDetailRequestSchema, type PluginDetail } from '../../generated/v1/plugin_pb';
import {
  ListClustersRequestSchema,
  ListInstallsRequestSchema,
  AddInstallRequestSchema,
  type ClusterSummary,
  type Install,
} from '../../generated/v1/cluster_pb';
import { firstValueFrom } from 'rxjs';
import { ToastService } from '../toast.service';

// Extended cluster type for UI state
interface ClusterWithState extends ClusterSummary {
  installed: boolean;
}

// Extended install type with cluster ID
interface InstallWithCluster extends Install {
  clusterId: string;
}

@Component({
  selector: 'app-plugin-details',
  standalone: true,
  imports: [
    CommonModule,
    RouterLink,
    InstallPluginModalComponent,
    NgIcon,
    LoadingIndicatorComponent,
  ],
  viewProviders: [
    provideIcons({
      tablerChevronRight,
      tablerCheck,
      tablerCircleXFill,
    }),
  ],
  templateUrl: './plugin-details.component.html',
})
export class PluginDetailsComponent implements OnInit {
  private titleService = inject(TitleService);
  private sanitizer = inject(DomSanitizer);
  private route = inject(ActivatedRoute);
  private pluginClient = inject(PLUGIN);
  private clusterClient = inject(CLUSTER);
  private toastService = inject(ToastService);

  pluginId = signal<string>('');
  plugin = signal<PluginDetail | null>(null);
  clusters = signal<ClusterWithState[]>([]);
  installs = signal<InstallWithCluster[]>([]);

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
      // Fetch plugin, clusters, and installs in parallel
      const [pluginResponse, clustersResponse] = await Promise.all([
        firstValueFrom(
          this.pluginClient.getPluginDetail(create(GetPluginDetailRequestSchema, { pluginId: id })),
        ),
        firstValueFrom(this.clusterClient.listClusters(create(ListClustersRequestSchema, {}))),
      ]);

      // Check if plugin was found
      if (!pluginResponse.plugin) {
        this.errorMessage.set('Plugin not found');
        this.isLoading.set(false);
        return;
      }

      this.plugin.set(pluginResponse.plugin);
      this.titleService.setTitle(`${pluginResponse.plugin.name} — Plugins`);

      // Fetch installs for all clusters
      const installsPromises = clustersResponse.clusters.map((cluster) =>
        firstValueFrom(
          this.clusterClient.listInstalls(
            create(ListInstallsRequestSchema, { clusterId: cluster.id }),
          ),
        ).then((response) => ({
          clusterId: cluster.id,
          installs: response.installs,
        })),
      );

      const installsResponses = await Promise.all(installsPromises);

      // Flatten all installs and augment with cluster ID
      const allInstalls: InstallWithCluster[] = installsResponses.flatMap(
        ({ clusterId, installs }) => installs.map((install) => ({ ...install, clusterId })),
      );
      this.installs.set(allInstalls);

      // Map clusters with install state
      this.clusters.set(
        clustersResponse.clusters.map((cluster) => ({
          ...cluster,
          installed: allInstalls.some(
            (install) => install.clusterId === cluster.id && install.pluginId === id,
          ),
        })),
      );

      this.isLoading.set(false);
    } catch (error) {
      console.error('Failed to load plugin details:', error);
      this.errorMessage.set(
        error instanceof Error ? error.message : 'Failed to load plugin details',
      );
      this.isLoading.set(false);
    }
  }

  getRenderedMarkdown(): SafeHtml {
    const description = this.plugin()?.description || '';

    // Simple markdown to HTML conversion
    let html = description
      .replace(/^# (.*$)/gim, '<h1 class="text-2xl font-bold mb-3 dark:text-white">$1</h1>')
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
    if (!cluster || cluster.installed) {
      return;
    }

    try {
      // Call the API to install the plugin
      const request = create(AddInstallRequestSchema, {
        clusterId: clusterId,
        pluginId: this.pluginId(),
      });

      await firstValueFrom(this.clusterClient.addInstall(request));

      // Update local state
      this.clusters.update((clusters) =>
        clusters.map((c) => (c.id === clusterId ? { ...c, installed: true } : c)),
      );

      this.toastService.success(
        `Plugin ${this.plugin()?.name} installed on cluster ${cluster.name}`,
      );
    } catch (error) {
      console.error('Failed to install plugin:', error);
      this.toastService.error(error instanceof Error ? error.message : 'Failed to install plugin');
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
