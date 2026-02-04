import { Component, inject, signal, OnInit, ChangeDetectionStrategy } from '@angular/core';
import { CommonModule } from '@angular/common';
import { RouterLink } from '@angular/router';
import { TitleService } from '../title.service';
import { InstallPluginModalComponent } from '../install-plugin-modal/install-plugin-modal';
import { NgIcon, provideIcons } from '@ng-icons/core';
import { tablerCheck, tablerHelpCircle } from '@ng-icons/tabler-icons';
import { LoadingIndicatorComponent } from '../icons';
import { BreadcrumbComponent } from '../breadcrumb/breadcrumb.component';
import { PLUGIN, CLUSTER } from '../../connect/tokens';
import { create } from '@bufbuild/protobuf';
import {
  ListPluginsRequestSchema,
  ListPresetsRequestSchema,
  type Category,
  type Preset,
  type PluginSummary,
} from '../../generated/v1/plugin_pb';
import {
  ListClustersRequestSchema,
  ListInstallsRequestSchema,
  AddInstallRequestSchema,
  InstallSchema,
  type ClusterSummary,
  type Install,
} from '../../generated/v1/cluster_pb';
import { firstValueFrom } from 'rxjs';
import { ToastService } from '../toast.service';

// Extended plugin type with presets array (computed from backend data)
interface PluginWithPresets extends Pick<
  PluginSummary,
  'id' | 'name' | 'descriptionShort' | 'description' | 'categories' | 'tags'
> {
  presets?: string[]; // Array of preset IDs this plugin belongs to
}

// Extended cluster type for UI state
interface ClusterWithState extends ClusterSummary {
  installed: boolean;
}

// Extended install type with cluster ID
interface InstallWithCluster extends Install {
  clusterId: string;
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
  imports: [
    CommonModule,
    RouterLink,
    InstallPluginModalComponent,
    NgIcon,
    LoadingIndicatorComponent,
    BreadcrumbComponent,
  ],
  viewProviders: [
    provideIcons({
      tablerCheck,
      tablerHelpCircle,
    }),
  ],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './plugins.component.html',
})
export class PluginsComponent implements OnInit {
  private titleService = inject(TitleService);
  private pluginClient = inject(PLUGIN);
  private clusterClient = inject(CLUSTER);
  private toastService = inject(ToastService);

  selectedCategory = 'all';
  selectedPreset = 'all';

  showInstallModal = false;
  selectedPlugin: PluginWithPresets | null = null;

  isLoading = signal(true);
  errorMessage = signal<string | null>(null);

  clusters: ClusterSummary[] = [];
  installs: InstallWithCluster[] = [];

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
      // Fetch plugins, presets, and clusters in parallel
      const [pluginsResponse, presetsResponse, clustersResponse] = await Promise.all([
        firstValueFrom(this.pluginClient.listPlugins(create(ListPluginsRequestSchema, {}))),
        firstValueFrom(this.pluginClient.listPresets(create(ListPresetsRequestSchema, {}))),
        firstValueFrom(this.clusterClient.listClusters(create(ListClustersRequestSchema, {}))),
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
          presets: assignedPresets,
        };
      });

      // Store clusters
      this.clusters = clustersResponse.clusters;

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
      this.installs = installsResponses.flatMap(({ clusterId, installs }) =>
        installs.map((install) => ({ ...install, clusterId })),
      );

      this.isLoading.set(false);
    } catch (error) {
      console.error('Failed to load data:', error);
      this.errorMessage.set(error instanceof Error ? error.message : 'Failed to load data');
      this.isLoading.set(false);
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
            existing.count++;
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
  get clustersForModal(): ClusterWithState[] {
    if (!this.selectedPlugin) {
      return [];
    }

    return this.clusters.map((cluster) => ({
      ...cluster,
      installed: this.installs.some(
        (install) =>
          install.clusterId === cluster.id && install.pluginId === this.selectedPlugin!.id,
      ),
    }));
  }

  onInstallPlugin(plugin: PluginWithPresets) {
    this.selectedPlugin = plugin;
    this.showInstallModal = true;
  }

  closeInstallModal(): void {
    this.showInstallModal = false;
    this.selectedPlugin = null;
  }

  async onInstallOnCluster(clusterId: string): Promise<void> {
    const cluster = this.clusters.find((c) => c.id === clusterId);
    if (!cluster || !this.selectedPlugin) {
      return;
    }

    // Check if already installed
    const alreadyInstalled = this.installs.some(
      (install) => install.clusterId === clusterId && install.pluginId === this.selectedPlugin!.id,
    );
    if (alreadyInstalled) {
      return;
    }

    try {
      // Call the API to install the plugin
      const request = create(AddInstallRequestSchema, {
        clusterId: clusterId,
        pluginId: this.selectedPlugin.id,
      });

      const response = await firstValueFrom(this.clusterClient.addInstall(request));

      // Add the new install to the local state (augmented with clusterId)
      const newInstall: InstallWithCluster = {
        ...create(InstallSchema, {
          id: response.installId,
          pluginId: this.selectedPlugin.id,
        }),
        clusterId: clusterId,
      };
      this.installs.push(newInstall);

      this.toastService.success(
        `Plugin ${this.selectedPlugin.name} installed on cluster ${cluster.name}`,
      );
    } catch (error) {
      console.error('Failed to install plugin:', error);
      this.toastService.error(error instanceof Error ? error.message : 'Failed to install plugin');
    }
  }

  getPluginIconName(pluginName: string): string {
    return pluginName.toLowerCase().replace(/[^a-z]+/g, '-');
  }
}
