import { Component, inject, signal, OnInit } from '@angular/core';
import { CommonModule } from '@angular/common';
import { RouterLink } from '@angular/router';
import { TitleService } from '../title.service';
import { InstallPluginModalComponent } from '../install-plugin-modal/install-plugin-modal';
import {
  CheckmarkIconComponent,
  QuestionCircleIconComponent,
  LoadingIndicatorComponent,
} from '../icons';
import { PLUGIN, CLUSTER } from '../../connect/tokens';
import { create } from '@bufbuild/protobuf';
import {
  ListPluginsRequestSchema,
  ListPresetsRequestSchema,
  type Category,
  type Preset,
  type Plugin,
} from '../../generated/v1/plugin_pb';
import { ListClustersRequestSchema, type ClusterSummary } from '../../generated/v1/cluster_pb';
import { firstValueFrom } from 'rxjs';

// Extended plugin type with presets array (computed from backend data)
interface PluginWithPresets extends Pick<
  Plugin,
  'id' | 'name' | 'description' | 'categories' | 'tags'
> {
  presets?: string[]; // Array of preset IDs this plugin belongs to
}

// Extended cluster type for UI state
interface ClusterWithState extends ClusterSummary {
  installed: boolean;
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
  standalone: true,
  imports: [
    CommonModule,
    RouterLink,
    InstallPluginModalComponent,
    CheckmarkIconComponent,
    QuestionCircleIconComponent,
    LoadingIndicatorComponent,
  ],
  templateUrl: './plugins.component.html',
})
export class PluginsComponent implements OnInit {
  private titleService = inject(TitleService);
  private pluginClient = inject(PLUGIN);
  private clusterClient = inject(CLUSTER);

  selectedCategory = 'all';
  selectedPreset = 'all';

  showInstallModal = false;
  selectedPlugin: PluginWithPresets | null = null;

  isLoading = signal(true);
  errorMessage = signal<string | null>(null);

  clusters: ClusterWithState[] = [];

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
          categories: backendPlugin.categories,
          tags: backendPlugin.tags,
          presets: assignedPresets,
        };
      });

      // Map clusters to include UI state
      this.clusters = clustersResponse.clusters.map((cluster) => ({
        ...cluster,
        installed: false,
      }));

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

  onInstallPlugin(plugin: PluginWithPresets) {
    this.selectedPlugin = plugin;
    this.showInstallModal = true;
  }

  closeInstallModal(): void {
    this.showInstallModal = false;
    this.selectedPlugin = null;
  }

  onInstallOnCluster(clusterId: string): void {
    const cluster = this.clusters.find((c) => c.id === clusterId);
    if (cluster && !cluster.installed) {
      cluster.installed = true;
    }
  }
}
