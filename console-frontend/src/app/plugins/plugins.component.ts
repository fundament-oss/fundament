import { Component, inject } from '@angular/core';
import { CommonModule } from '@angular/common';
import { Title } from '@angular/platform-browser';
import { RouterLink } from '@angular/router';
import { InstallPluginModalComponent } from '../install-plugin-modal/install-plugin-modal';
import { CheckmarkIconComponent, CloseIconComponent } from '../icons';

export interface Plugin {
  id: string;
  title: string;
  description: string;
  category: string;
  isOfficial?: boolean;
  presets?: string[]; // Array of preset IDs this plugin belongs to
}

interface Cluster {
  id: string;
  name: string;
  installed: boolean;
}

interface Category {
  id: string;
  name: string;
  count: number;
}

interface Preset {
  id: string;
  name: string;
  description: string;
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
    CloseIconComponent,
  ],
  templateUrl: './plugins.component.html',
})
export class PluginsComponent {
  private titleService = inject(Title);

  selectedCategory = 'all';
  selectedPreset = 'all';

  showInstallModal = false;
  selectedPlugin: Plugin | null = null;

  clusters: Cluster[] = [
    { id: 'cluster-1', name: 'cluster-1', installed: false },
    { id: 'cluster-2', name: 'cluster-2', installed: false },
    { id: 'cluster-3', name: 'cluster-3', installed: false },
    { id: 'cluster-4', name: 'cluster-4', installed: false },
  ];

  get presets(): Preset[] {
    const presetCounts = new Map<string, number>();

    // Count plugins per preset based on current category filter
    this.plugins.forEach((plugin) => {
      const matchesCategory =
        this.selectedCategory === 'all' || plugin.category === this.selectedCategory;

      if (matchesCategory && plugin.presets) {
        plugin.presets.forEach((presetId) => {
          presetCounts.set(presetId, (presetCounts.get(presetId) || 0) + 1);
        });
      }
    });

    // Count all plugins for 'all' preset
    const allCount = this.plugins.filter(
      (plugin) => this.selectedCategory === 'all' || plugin.category === this.selectedCategory,
    ).length;

    return [
      { id: 'all', name: 'All presets', description: 'Show all plugins', count: allCount },
      {
        id: 'havenplus',
        name: 'Haven+ preset',
        description: 'Includes monitoring, logging, security scanning, and backup solutions.',
        count: presetCounts.get('havenplus') || 0,
      },
      {
        id: 'preset2',
        name: 'Preset #2',
        description: 'Some other preset.',
        count: presetCounts.get('preset2') || 0,
      },
    ];
  }

  plugins: Plugin[] = [
    {
      id: 'alloy',
      title: 'Alloy',
      description:
        'Collects, processes, and sends logs, metrics, and traces (telemetry) to observability tools.',
      category: 'observability',
      isOfficial: true,
      presets: ['havenplus'],
    },
    {
      id: 'cert-manager',
      title: 'cert-manager',
      description: 'Automates the requesting and renewal of TLS certificates.',
      category: 'security',
      isOfficial: true,
      presets: ['havenplus'],
    },
    {
      id: 'cloudnative-pg',
      title: 'Cloudnative-pg',
      description: 'Manages PostgreSQL clusters on Kubernetes.',
      category: 'databases',
      isOfficial: true,
      presets: ['havenplus'],
    },
    {
      id: 'eck-operator',
      title: 'ECK operator',
      description: 'Manages Elasticsearch clusters and associated components within Kubernetes.',
      category: 'databases',
      isOfficial: true,
      presets: ['havenplus'],
    },
    {
      id: 'grafana',
      title: 'Grafana',
      description: 'Visualizes metrics, logs, and traces in clear dashboards.',
      category: 'observability',
      isOfficial: true,
      presets: ['havenplus', 'preset2'],
    },
    {
      id: 'istio-gateway',
      title: 'Istio gateway',
      description: 'Manages incoming traffic to services via configurable ingress policies.',
      category: 'networking',
      isOfficial: true,
      presets: ['havenplus'],
    },
    {
      id: 'istio',
      title: 'Istio',
      description:
        'Controls service-to-service communication, security, and observability within a service mesh.',
      category: 'networking',
      isOfficial: true,
      presets: ['havenplus'],
    },
    {
      id: 'keycloak',
      title: 'Keycloak',
      description:
        'Provides identity and access management with support for SSO, OpenID Connect, and more.',
      category: 'security',
      isOfficial: true,
      presets: ['havenplus'],
    },
    {
      id: 'loki',
      title: 'Loki',
      description: 'Stores log files and makes them searchable.',
      category: 'observability',
      isOfficial: true,
      presets: ['havenplus'],
    },
    {
      id: 'mimir',
      title: 'Mimir',
      description: 'Stores time series (metrics) in a scalable way.',
      category: 'observability',
      isOfficial: true,
      presets: ['havenplus'],
    },
    {
      id: 'pinniped',
      title: 'Pinniped',
      description:
        'Provides secure authentication in Kubernetes environments via existing identity providers.',
      category: 'security',
      isOfficial: true,
      presets: ['havenplus', 'preset2'],
    },
    {
      id: 'sealed-secrets',
      title: 'Sealed secrets',
      description: 'Enables encrypted secrets to be safely stored in Git.',
      category: 'security',
      isOfficial: true,
      presets: ['havenplus'],
    },
    {
      id: 'tempo',
      title: 'Tempo',
      description:
        'Processes and visualizes tracing data to make dependencies and performance insights clear.',
      category: 'observability',
      isOfficial: true,
      presets: ['havenplus'],
    },
  ];

  get categories(): Category[] {
    const categoryMap = new Map<string, number>();

    // Count plugins per category based on current preset filter
    this.plugins.forEach((plugin) => {
      const matchesPreset =
        this.selectedPreset === 'all' ||
        (plugin.presets && plugin.presets.includes(this.selectedPreset));

      if (matchesPreset) {
        categoryMap.set(plugin.category, (categoryMap.get(plugin.category) || 0) + 1);
      }
    });

    // Count all plugins for 'all' category
    const allCount = this.plugins.filter(
      (plugin) =>
        this.selectedPreset === 'all' ||
        (plugin.presets && plugin.presets.includes(this.selectedPreset)),
    ).length;

    // Create categories array with dynamic counts
    const categories: Category[] = [{ id: 'all', name: 'All categories', count: allCount }];

    // Add categories based on actual plugin categories
    const categoryNames: Record<string, string> = {
      observability: 'Observability',
      security: 'Security',
      databases: 'Databases',
      networking: 'Networking',
    };

    categoryMap.forEach((count, categoryId) => {
      categories.push({
        id: categoryId,
        name: categoryNames[categoryId] || categoryId,
        count: count,
      });
    });

    return categories;
  }

  constructor() {
    this.titleService.setTitle('Plugins — Fundament Console');
  }

  get filteredPlugins(): Plugin[] {
    return this.plugins.filter((plugin) => {
      // Filter by preset
      const matchesPreset =
        this.selectedPreset === 'all' ||
        (plugin.presets && plugin.presets.includes(this.selectedPreset));

      // Filter by category
      const matchesCategory =
        this.selectedCategory === 'all' || plugin.category === this.selectedCategory;

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

  onInstallPlugin(plugin: Plugin) {
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
