import {
  Component,
  inject,
  computed,
  signal,
  OnInit,
  ChangeDetectionStrategy,
} from '@angular/core';
import { CommonModule } from '@angular/common';
import { Router, RouterLink } from '@angular/router';
import { NgIcon, provideIcons } from '@ng-icons/core';
import { tablerCircleXFill } from '@ng-icons/tabler-icons/fill';
import { create } from '@bufbuild/protobuf';
import { firstValueFrom } from 'rxjs';
import { TitleService } from '../title.service';
import { ToastService } from '../toast.service';
import { ClusterWizardStateService } from '../add-cluster-wizard-layout/cluster-wizard-state.service';
import { CLUSTER, PLUGIN } from '../../connect/tokens';
import {
  CreateClusterRequestSchema,
  CreateNodePoolRequestSchema,
  AddInstallRequestSchema,
} from '../../generated/v1/cluster_pb';
import {
  ListPluginsRequestSchema,
  ListPresetsRequestSchema,
  type PluginSummary,
  type Preset,
} from '../../generated/v1/plugin_pb';

@Component({
  selector: 'app-add-cluster-summary',
  imports: [CommonModule, RouterLink, NgIcon],
  viewProviders: [
    provideIcons({
      tablerCircleXFill,
    }),
  ],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './add-cluster-summary.component.html',
})
export default class AddClusterSummaryComponent implements OnInit {
  private titleService = inject(TitleService);

  private router = inject(Router);

  private client = inject(CLUSTER);

  private pluginClient = inject(PLUGIN);

  private toastService = inject(ToastService);

  protected stateService = inject(ClusterWizardStateService);

  // Computed signal to access state
  protected state = computed(() => this.stateService.getState());

  // Error state
  protected errorMessage = signal<string | null>(null);

  protected isCreating = signal<boolean>(false);

  // Plugin and preset data
  protected plugins = signal<PluginSummary[]>([]);

  protected presets = signal<Preset[]>([]);

  protected isLoadingPlugins = signal<boolean>(true);

  constructor() {
    this.titleService.setTitle('Cluster summary');
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

  async onCreateCluster() {
    const wizardState = this.state();

    // Validate required fields
    if (!wizardState.clusterName || !wizardState.region || !wizardState.kubernetesVersion) {
      this.errorMessage.set('Missing required cluster information');
      return;
    }

    // Clear previous errors and set loading state
    this.errorMessage.set(null);
    this.isCreating.set(true);

    try {
      // Build the request
      const request = create(CreateClusterRequestSchema, {
        name: wizardState.clusterName,
        region: wizardState.region,
        kubernetesVersion: wizardState.kubernetesVersion,
      });

      // Call the API to create the cluster
      const response = await firstValueFrom(this.client.createCluster(request));

      // Create node pools if any are configured
      if (wizardState.nodePools && wizardState.nodePools.length > 0) {
        await Promise.all(
          wizardState.nodePools.map((pool) => {
            const nodePoolRequest = create(CreateNodePoolRequestSchema, {
              clusterId: response.clusterId,
              name: pool.name,
              machineType: pool.machineType,
              autoscaleMin: pool.autoscaleMin,
              autoscaleMax: pool.autoscaleMax,
            });
            return firstValueFrom(this.client.createNodePool(nodePoolRequest));
          }),
        );
      }

      // Install plugins if any are configured
      if (wizardState.plugins && wizardState.plugins.length > 0) {
        await Promise.all(
          wizardState.plugins.map((pluginId) => {
            const installRequest = create(AddInstallRequestSchema, {
              clusterId: response.clusterId,
              pluginId,
            });
            return firstValueFrom(this.client.addInstall(installRequest));
          }),
        );
      }

      // Reset wizard state
      this.stateService.reset();

      // Set toast message
      this.toastService.success('Your cluster is being provisioned. This may take a few minutes.');

      // Navigate to the cluster detail page
      this.router.navigate(['/clusters', response.clusterId]);
    } catch (error) {
      this.errorMessage.set(
        error instanceof Error
          ? `Failed to create cluster: ${error.message}`
          : 'Failed to create cluster. Please try again.',
      );
    } finally {
      this.isCreating.set(false);
    }
  }
}
