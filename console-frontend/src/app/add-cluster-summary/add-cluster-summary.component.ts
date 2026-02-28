import {
  Component,
  inject,
  computed,
  signal,
  OnInit,
  OnDestroy,
  ChangeDetectionStrategy,
} from '@angular/core';
import { Router, RouterLink } from '@angular/router';
import { NgIcon, provideIcons } from '@ng-icons/core';
import { tablerCircleCheck, tablerArrowBackUp } from '@ng-icons/tabler-icons';
import { tablerCircleXFill } from '@ng-icons/tabler-icons/fill';
import { create } from '@bufbuild/protobuf';
import { firstValueFrom } from 'rxjs';
import { TitleService } from '../title.service';
import { ClusterWizardStateService } from '../add-cluster-wizard-layout/cluster-wizard-state.service';
import { CLUSTER, PLUGIN } from '../../connect/tokens';
import {
  CreateClusterRequestSchema,
  CreateNodePoolRequestSchema,
  AddInstallRequestSchema,
  GetClusterRequestSchema,
  GetNodePoolRequestSchema,
} from '../../generated/v1/cluster_pb';
import {
  ListPluginsRequestSchema,
  ListPresetsRequestSchema,
  type PluginSummary,
  type Preset,
} from '../../generated/v1/plugin_pb';
import { NodePoolStatus } from '../../generated/v1/common_pb';
import ModalComponent from '../modal/modal.component';
import LoadingIndicatorComponent from '../icons/loading-indicator.component';

interface ProgressItem {
  key: string;
  type: 'cluster' | 'nodepool' | 'plugin';
  name: string;
  requestStatus: 'pending' | 'in_progress' | 'succeeded' | 'failed';
  syncStatus: 'none' | 'syncing' | 'synced' | 'failed';
  error?: string;
  shootStatus?: string;
  nodePoolConfig?: {
    name: string;
    machineType: string;
    autoscaleMin: number;
    autoscaleMax: number;
  };
  pluginId?: string;
  createdId?: string;
}

@Component({
  selector: 'app-add-cluster-summary',
  imports: [RouterLink, NgIcon, ModalComponent, LoadingIndicatorComponent],
  viewProviders: [
    provideIcons({
      tablerCircleXFill,
      tablerCircleCheck,
      tablerArrowBackUp,
    }),
  ],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './add-cluster-summary.component.html',
})
export default class AddClusterSummaryComponent implements OnInit, OnDestroy {
  private titleService = inject(TitleService);

  private router = inject(Router);

  private client = inject(CLUSTER);

  private pluginClient = inject(PLUGIN);

  protected stateService = inject(ClusterWizardStateService);

  protected state = computed(() => this.stateService.getState());

  protected errorMessage = signal<string | null>(null);

  protected isCreating = signal<boolean>(false);

  // Plugin and preset data
  protected plugins = signal<PluginSummary[]>([]);

  protected presets = signal<Preset[]>([]);

  protected isLoadingPlugins = signal<boolean>(true);

  // Modal state
  protected showModal = signal(false);

  protected progressItems = signal<ProgressItem[]>([]);

  protected clusterId = signal<string | null>(null);

  private clusterConfig?: { name: string; region: string; kubernetesVersion: string };

  private pollInterval?: ReturnType<typeof setInterval>;

  constructor() {
    this.titleService.setTitle('Cluster summary');
  }

  ngOnDestroy() {
    this.stopPolling();
  }

  async ngOnInit() {
    try {
      // Fetch plugins and presets to display names
      const [pluginsResponse, presetsResponse] = await Promise.all([
        firstValueFrom(this.pluginClient.listPlugins(create(ListPluginsRequestSchema, {}))),
        firstValueFrom(this.pluginClient.listPresets(create(ListPresetsRequestSchema, {}))),
      ]);

      this.plugins.set(pluginsResponse.plugins);
      this.presets.set(presetsResponse.presets);
    } catch (error) {
      // eslint-disable-next-line no-console
      console.error('Failed to load plugins and presets:', error);
    } finally {
      this.isLoadingPlugins.set(false);
    }
  }

  // Helper to get preset name
  getPresetName(presetId: string): string {
    if (presetId === 'custom') {
      return 'Custom plugin selection';
    }
    const preset = this.presets().find((p) => p.id === presetId);
    return preset?.name || presetId;
  }

  // Helper to get plugin names
  getPluginNames(pluginIds: string[]): string[] {
    return pluginIds
      .map((id) => {
        const plugin = this.plugins().find((p) => p.id === id);
        return plugin?.name || id;
      })
      .sort();
  }

  private getPluginName(pluginId: string): string {
    const plugin = this.plugins().find((p) => p.id === pluginId);
    return plugin?.name || pluginId;
  }

  private updateItem(key: string, updates: Partial<ProgressItem>) {
    this.progressItems.update((items) =>
      items.map((item) => (item.key === key ? { ...item, ...updates } : item)),
    );
  }

  async onCreateCluster() {
    const wizardState = this.state();

    // Validate required fields
    if (
      !wizardState.clusterSlug ||
      !wizardState.region ||
      !wizardState.kubernetesVersion
    ) {
      this.errorMessage.set('Missing required cluster information');
      return;
    }

    // Save cluster config for retries
    this.clusterConfig = {
      name: wizardState.clusterSlug,
      region: wizardState.region,
      kubernetesVersion: wizardState.kubernetesVersion,
    };

    // Clear previous errors and set loading state
    this.errorMessage.set(null);
    this.isCreating.set(true);

    // Build progress items
    const items: ProgressItem[] = [
      {
        key: 'cluster',
        type: 'cluster',
        name: 'Cluster creation',
        requestStatus: 'pending',
        syncStatus: 'none',
      },
    ];

    if (wizardState.nodePools) {
      items.push(
        ...wizardState.nodePools.map((pool) => ({
          key: `nodepool-${pool.name}`,
          type: 'nodepool' as const,
          name: pool.name,
          requestStatus: 'pending' as const,
          syncStatus: 'none' as const,
          nodePoolConfig: pool,
        })),
      );
    }

    if (wizardState.plugins) {
      items.push(
        ...wizardState.plugins.map((pluginId) => ({
          key: `plugin-${pluginId}`,
          type: 'plugin' as const,
          name: this.getPluginName(pluginId),
          requestStatus: 'pending' as const,
          syncStatus: 'none' as const,
          pluginId,
        })),
      );
    }

    this.progressItems.set(items);
    this.showModal.set(true);

    await this.executeCreation();
  }

  private async executeCreation() {
    if (!this.clusterConfig) return;

    // Step 1: Create cluster
    this.updateItem('cluster', { requestStatus: 'in_progress' });

    try {
      const request = create(CreateClusterRequestSchema, {
        name: this.clusterConfig.name,
        region: this.clusterConfig.region,
        kubernetesVersion: this.clusterConfig.kubernetesVersion,
      });

      const response = await firstValueFrom(this.client.createCluster(request));
      this.clusterId.set(response.clusterId);
      this.updateItem('cluster', {
        requestStatus: 'succeeded',
        syncStatus: 'syncing',
        createdId: response.clusterId,
      });

      // Reset wizard state since we have the cluster now
      this.stateService.reset();
    } catch (error) {
      this.updateItem('cluster', {
        requestStatus: 'failed',
        error: error instanceof Error ? error.message : 'Failed to create cluster',
      });
      this.isCreating.set(false);
      return;
    }

    const cid = this.clusterId()!;

    // Step 2: Create node pools and install plugins in parallel
    const nodePoolItems = this.progressItems().filter(
      (item) => item.type === 'nodepool' && item.nodePoolConfig,
    );
    const pluginItems = this.progressItems().filter(
      (item) => item.type === 'plugin' && item.pluginId,
    );

    await Promise.allSettled([
      ...nodePoolItems.map((item) => this.createNodePool(item.key, item.nodePoolConfig!, cid)),
      ...pluginItems.map((item) => this.installPlugin(item.key, item.pluginId!, cid)),
    ]);

    // Start polling for sync status
    this.startPolling();
    this.isCreating.set(false);
  }

  private async createNodePool(
    key: string,
    config: { name: string; machineType: string; autoscaleMin: number; autoscaleMax: number },
    clusterId?: string,
  ) {
    const cid = clusterId || this.clusterId();
    if (!cid) return;

    this.updateItem(key, { requestStatus: 'in_progress', error: undefined });

    try {
      const request = create(CreateNodePoolRequestSchema, {
        clusterId: cid,
        name: config.name,
        machineType: config.machineType,
        autoscaleMin: config.autoscaleMin,
        autoscaleMax: config.autoscaleMax,
      });

      const response = await firstValueFrom(this.client.createNodePool(request));
      this.updateItem(key, {
        requestStatus: 'succeeded',
        syncStatus: 'syncing',
        createdId: response.nodePoolId,
      });
    } catch (error) {
      this.updateItem(key, {
        requestStatus: 'failed',
        error: error instanceof Error ? error.message : 'Failed to create node pool',
      });
    }
  }

  private async installPlugin(key: string, pluginId: string, clusterId?: string) {
    const cid = clusterId || this.clusterId();
    if (!cid) return;

    this.updateItem(key, { requestStatus: 'in_progress', error: undefined });

    try {
      const request = create(AddInstallRequestSchema, {
        clusterId: cid,
        pluginId,
      });
      await firstValueFrom(this.client.addInstall(request));
      this.updateItem(key, { requestStatus: 'succeeded' });
    } catch (error) {
      this.updateItem(key, {
        requestStatus: 'failed',
        error: error instanceof Error ? error.message : 'Failed to install plugin',
      });
    }
  }

  private startPolling() {
    this.stopPolling();
    // Poll immediately, then every 5 seconds
    this.pollSyncStatus();
    this.pollInterval = setInterval(() => this.pollSyncStatus(), 5000);
  }

  private stopPolling() {
    if (this.pollInterval) {
      clearInterval(this.pollInterval);
      this.pollInterval = undefined;
    }
  }

  private async pollSyncStatus() {
    const cid = this.clusterId();
    if (!cid) return;

    // Poll cluster sync status
    const clusterItem = this.progressItems().find((i) => i.key === 'cluster');
    if (clusterItem && clusterItem.syncStatus === 'syncing') {
      try {
        const response = await firstValueFrom(
          this.client.getCluster(create(GetClusterRequestSchema, { clusterId: cid })),
        );
        const syncState = response.cluster?.syncState;

        if (syncState?.shootStatus === 'Ready' || syncState?.shootStatus === 'ready') {
          this.updateItem('cluster', { syncStatus: 'synced', shootStatus: 'Ready' });
        } else if (syncState?.syncError) {
          this.updateItem('cluster', {
            syncStatus: 'failed',
            error: syncState.syncError,
            shootStatus: syncState.shootStatus || undefined,
          });
        } else {
          this.updateItem('cluster', {
            shootStatus: syncState?.shootStatus || 'Pending',
          });
        }
      } catch {
        // Ignore polling errors
      }
    }

    // Poll node pool sync status
    await Promise.all(
      this.progressItems()
        .filter(
          (item) => item.type === 'nodepool' && item.syncStatus === 'syncing' && item.createdId,
        )
        .map(async (item) => {
          try {
            const response = await firstValueFrom(
              this.client.getNodePool(
                create(GetNodePoolRequestSchema, { nodePoolId: item.createdId! }),
              ),
            );
            const status = response.nodePool?.status;

            if (status === NodePoolStatus.HEALTHY) {
              this.updateItem(item.key, { syncStatus: 'synced' });
            } else if (status === NodePoolStatus.UNHEALTHY) {
              this.updateItem(item.key, {
                syncStatus: 'failed',
                error: 'Node pool is unhealthy',
              });
            }
          } catch {
            // Ignore polling errors
          }
        }),
    );

    // Stop polling when all syncing items are done
    const hasSyncing = this.progressItems().some((item) => item.syncStatus === 'syncing');
    if (!hasSyncing) {
      this.stopPolling();
    }
  }

  protected async retryItem(key: string) {
    const item = this.progressItems().find((i) => i.key === key);
    if (!item) return;

    if (item.type === 'cluster') {
      // Reset all items and restart the entire creation
      this.progressItems.update((items) =>
        items.map((i) => ({
          ...i,
          requestStatus: 'pending' as const,
          syncStatus: 'none' as const,
          error: undefined,
          shootStatus: undefined,
          createdId: undefined,
        })),
      );
      this.clusterId.set(null);
      this.isCreating.set(true);
      await this.executeCreation();
    } else if (item.type === 'nodepool' && item.nodePoolConfig) {
      await this.createNodePool(key, item.nodePoolConfig);
      if (!this.pollInterval && this.progressItems().some((i) => i.syncStatus === 'syncing')) {
        this.startPolling();
      }
    } else if (item.type === 'plugin' && item.pluginId) {
      await this.installPlugin(key, item.pluginId);
    }
  }

  protected onModalClose() {
    // Don't allow closing while requests are in progress
    const hasInProgress = this.progressItems().some((i) => i.requestStatus === 'in_progress');
    if (!hasInProgress) {
      this.showModal.set(false);
      this.stopPolling();
    }
  }

  protected navigateToCluster() {
    const cid = this.clusterId();
    if (cid) {
      this.router.navigate(['/clusters', cid]);
    }
  }
}
