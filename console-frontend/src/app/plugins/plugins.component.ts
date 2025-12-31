import { Component, inject } from '@angular/core';
import { CommonModule } from '@angular/common';
import { Title, DomSanitizer, SafeHtml } from '@angular/platform-browser';
import { RouterLink } from '@angular/router';
import { InstallPluginModalComponent } from '../install-plugin-modal/install-plugin-modal';

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
  imports: [CommonModule, RouterLink, InstallPluginModalComponent],
  templateUrl: './plugins.component.html',
})
export class PluginsComponent {
  private titleService = inject(Title);
  private sanitizer = inject(DomSanitizer);

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

  getPluginLogo(logoName: string): SafeHtml {
    // Return SVG content for different logos
    const logos: Record<string, string> = {
      postgresql: `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><g fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="2"><path d="M4 6a8 3 0 1 0 16 0A8 3 0 1 0 4 6"/><path d="M4 6v6a8 3 0 0 0 16 0V6"/><path d="M4 12v6a8 3 0 0 0 16 0v-6"/></g></svg>`,
      'eck-operator': `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><g fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="2"><path d="M14 2a5 5 0 0 1 5 5c0 .712-.232 1.387-.5 2c1.894.042 3.5 1.595 3.5 3.5c0 1.869-1.656 3.4-3.5 3.5q.5.938.5 1.5a2.5 2.5 0 0 1-2.5 2.5c-.787 0-1.542-.432-2-1c-.786 1.73-2.476 3-4.5 3a5 5 0 0 1-4.583-7a3.5 3.5 0 0 1-.11-6.992h.195a2.5 2.5 0 0 1 2-4c.787 0 1.542.432 2 1c.786-1.73 2.476-3 4.5-3zM8.5 9l-3-1"/><path d="m9.5 5l-1 4l1 2l5 2l4-4m-.001 7l-3-.5l-1-2.5m.001 6l1-3.5M5.417 15L9.5 11"/></g></svg>`,
      grafana: `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 128 128"><linearGradient id="SVGDkkdxczn" x1="45.842" x2="45.842" y1="89.57" y2="8.802" gradientTransform="translate(-2.405 27.316)scale(1.4463)" gradientUnits="userSpaceOnUse"><stop offset="0" stop-color="#fcee1f"/><stop offset="1" stop-color="#f15b2a"/></linearGradient><path fill="url(#SVGDkkdxczn)" d="M69.162 0c-9.91 6.4-11.77 14.865-11.77 14.865s.002.206-.101.412c-.62.104-1.033.31-1.549.413c-.722.206-1.445.413-2.168.826l-2.168.93c-1.445.722-2.89 1.341-4.336 2.167c-1.342.826-2.683 1.548-4.025 2.477a1.3 1.3 0 0 1-.309-.205c-13.316-5.161-25.084 1.031-25.084 1.031c-1.032 14.245 5.367 23.02 6.606 24.672c-.31.929-.62 1.754-.93 2.58a53 53 0 0 0-2.166 9.91c-.103.413-.104 1.033-.207 1.445C8.671 67.613 5.06 80.103 5.06 80.103c10.219 11.768 22.193 12.49 22.193 12.49c1.445 2.685 3.302 5.369 5.264 7.743c.825 1.032 1.756 1.96 2.582 2.992c-3.716 10.632.619 19.613.619 19.613c11.458.413 18.992-4.955 20.54-6.297c1.136.31 2.272.724 3.407 1.034a47.3 47.3 0 0 0 10.633 1.549h4.644C80.31 126.969 89.807 128 89.807 128c6.71-7.123 7.123-14.038 7.123-15.69v-.62c1.342-1.033 2.683-2.064 4.129-3.2c2.684-2.374 4.955-5.264 7.02-8.154c.206-.207.309-.62.618-.826c7.639.413 12.903-4.748 12.903-4.748c-1.24-7.949-5.78-11.768-6.71-12.49l-.103-.104l-.103-.104l-.104-.103c0-.413.104-.93.104-1.445c.103-.93.103-1.755.103-2.58v-3.407c0-.206 0-.413-.103-.722l-.104-.723l-.103-.723c-.104-.929-.31-1.754-.413-2.58c-.825-3.406-2.166-6.71-3.818-9.498c-1.858-2.993-4.026-5.471-6.504-7.742c-2.477-2.168-5.264-4.025-8.154-5.264c-2.994-1.342-5.884-2.167-8.98-2.476c-1.446-.207-3.098-.207-4.544-.207H79.69c-.825.103-1.546.205-2.27.308c-3.096.62-5.883 1.756-8.36 3.201c-2.478 1.446-4.646 3.407-6.504 5.575c-1.858 2.167-3.2 4.438-4.13 6.916a23.3 23.3 0 0 0-1.548 7.431v2.684c0 .31 0 .62.104.93c.103 1.238.31 2.374.722 3.51c.723 2.27 1.756 4.334 3.098 6.09a20 20 0 0 0 4.54 4.335c1.756 1.136 3.408 1.96 5.266 2.477s3.509.826 5.16.722h2.376c.206 0 .412-.101.619-.101c.206 0 .31-.104.619-.104c.31-.103.825-.207 1.135-.31c.722-.207 1.342-.62 2.064-.826c.723-.31 1.24-.722 1.756-1.032c.103-.103.309-.207.412-.31c.62-.413.723-1.238.207-1.858c-.413-.413-1.136-.62-1.756-.31c-.103.103-.205.104-.412.207c-.413.206-1.032.413-1.445.619c-.62.103-1.135.31-1.754.414c-.31 0-.62.102-.93.102h-2.58c-.103 0-.31.001-.414-.102c-1.239-.206-2.58-.62-3.818-1.137c-1.239-.619-2.478-1.34-3.51-2.373a15.9 15.9 0 0 1-2.89-3.51c-.826-1.341-1.24-2.89-1.446-4.335c-.103-.826-.207-1.55-.103-2.375v-1.239c0-.413.103-.825.207-1.238c.619-3.406 2.27-6.71 4.851-9.187c.723-.723 1.342-1.238 2.168-1.754c.826-.62 1.547-1.032 2.373-1.342s1.756-.723 2.582-.93c.93-.206 1.858-.414 2.684-.414c.413 0 .929-.101 1.342-.101h1.238c1.032.103 2.065.205 2.994.412c1.961.413 3.82 1.135 5.678 2.168c3.613 2.064 6.708 5.16 8.566 8.877c.93 1.858 1.548 3.82 1.961 5.988c.103.62.104 1.03.207 1.547v2.787c0 .62-.103 1.136-.103 1.756c-.104.62-.102 1.134-.205 1.754c-.104.619-.208 1.136-.311 1.755c-.206 1.136-.722 2.168-1.031 3.303c-.826 2.168-1.963 4.232-3.305 5.986c-2.684 3.717-6.502 6.815-10.63 8.776c-2.169.929-4.337 1.755-6.608 2.064a19 19 0 0 1-3.407.309h-1.755c-.62 0-1.238.002-1.858-.102c-2.477-.206-4.85-.724-7.224-1.343c-2.375-.723-4.647-1.548-6.815-2.684c-4.335-2.27-8.153-5.573-11.25-9.289c-1.445-1.961-2.892-4.027-4.027-6.092s-1.961-4.438-2.58-6.709c-.723-2.27-1.032-4.645-1.135-7.02v-3.613c0-1.135.102-2.372.309-3.61c.103-1.24.309-2.376.619-3.614c.206-1.239.62-2.375.93-3.614c.722-2.374 1.444-4.644 2.476-6.812c2.064-4.335 4.645-8.155 7.742-11.252a25 25 0 0 1 2.479-2.168c.31-.31 1.135-1.033 2.064-1.549s1.858-1.136 2.89-1.549c.414-.206.93-.413 1.446-.722c.206-.103.411-.206.824-.309c.207-.103.414-.207.826-.31c1.033-.413 2.066-.825 3.098-1.135c.207-.103.62-.104.826-.207c.207-.103.618-.102.824-.205c.62-.103 1.033-.208 1.55-.414c.206-.104.619-.104.825-.207c.207 0 .62-.102.827-.102s.62-.103.826-.103l.412-.104l.412-.103c.206 0 .62-.104.826-.104c.31 0 .62-.104.93-.104c.206 0 .721-.101.928-.101c.206 0 .311 0 .62-.104h.723c.31 0 .618 0 .928-.103h4.647c2.064.103 4.128.31 5.986.723c3.82.722 7.638 1.961 10.941 3.613c3.304 1.548 6.4 3.611 8.877 5.78c.104.102.311.207.414.413c.104.103.31.206.412.412c.31.207.62.62.93.826c.31.207.62.62.93.827c.206.31.618.618.824.927c1.136 1.136 2.169 2.375 3.098 3.51a41.4 41.4 0 0 1 4.44 7.02c.102.103.1.207.204.414c.103.103.104.205.207.412c.103.206.206.62.412.826c.104.206.208.62.31.826c.104.207.208.62.311.826c.413 1.033.826 2.064 1.135 3.096c.62 1.548.929 2.993 1.239 4.13c.103.412.62.825 1.033.825c.619 0 .927-.414.927-1.033c-.31-1.755-.308-3.198-.412-4.953q-.31-3.253-1.238-7.434c-.62-2.787-1.86-5.677-3.305-8.877c-1.548-3.096-3.509-6.4-6.09-9.394c-1.032-1.239-2.167-2.373-3.302-3.612c1.858-7.122-2.168-13.42-2.168-13.42c-6.916-.412-11.253 2.168-12.801 3.303c-.206-.103-.618-.205-.824-.308c-1.136-.413-2.375-.93-3.613-1.342c-1.24-.31-2.478-.827-3.717-1.033c-1.239-.31-2.58-.62-4.026-.827c-.206 0-.413-.103-.722-.103C77.833 4.128 69.162 0 69.162 0"/></svg>`,
      'cert-manager': `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><g fill="none" stroke-linecap="round" stroke-linejoin="round" stroke-width="2"><path d="M0 0h24v24H0z"/><path fill="currentColor" d="M12.01 2.011a3.2 3.2 0 0 1 2.113.797l.154.145l.698.698a1.2 1.2 0 0 0 .71.341L15.82 4h1a3.2 3.2 0 0 1 3.195 3.018l.005.182v1c0 .27.092.533.258.743l.09.1l.697.698a3.2 3.2 0 0 1 .147 4.382l-.145.154l-.698.698a1.2 1.2 0 0 0-.341.71l-.008.135v1a3.2 3.2 0 0 1-3.018 3.195l-.182.005h-1a1.2 1.2 0 0 0-.743.258l-.1.09l-.698.697a3.2 3.2 0 0 1-4.382.147l-.154-.145l-.698-.698a1.2 1.2 0 0 0-.71-.341L8.2 20.02h-1a3.2 3.2 0 0 1-3.195-3.018L4 16.82v-1a1.2 1.2 0 0 0-.258-.743l-.09-.1l-.697-.698a3.2 3.2 0 0 1-.147-4.382l.145-.154l.698-.698a1.2 1.2 0 0 0 .341-.71L4 8.2v-1l.005-.182a3.2 3.2 0 0 1 3.013-3.013L7.2 4h1a1.2 1.2 0 0 0 .743-.258l.1-.09l.698-.697a3.2 3.2 0 0 1 2.269-.944m3.697 7.282a1 1 0 0 0-1.414 0L11 12.585l-1.293-1.292l-.094-.083a1 1 0 0 0-1.32 1.497l2 2l.094.083a1 1 0 0 0 1.32-.083l4-4l.083-.094a1 1 0 0 0-.083-1.32"/></g></svg>`,
      alloy: `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path fill="#ff6f00" d="M20.173 8.483a14.7 14.7 0 0 0-3.287-3.92l-.025-.02a13 13 0 0 0-.784-.603C14.28 2.67 12.317 2 10.394 2C7.953 2 5.741 3.302 4.167 5.668C1.952 8.994 1.48 13.656 2.99 17.269c1.134 2.712 4.077 4.47 7.873 4.706q.415.024.833.025c1.757 0 3.531-.338 5.073-.975c1.962-.81 3.463-2.048 4.342-3.583c1.304-2.28.945-5.712-.938-8.96zm-8.871.508c.863 0 1.723.354 2.341 1.048c.558.625.839 1.43.79 2.266a3.1 3.1 0 0 1-1.007 2.128l-.072.064a3.14 3.14 0 0 1-3.725.28a4.4 4.4 0 0 1-.745-.67a3 3 0 0 1-.17-.214a3.1 3.1 0 0 1-.416-.874l-.016-.057l-.002-.007c-.277-1.08.04-2.339.905-3.138l.066-.061a3.12 3.12 0 0 1 2.05-.764zm-.908-5.84c1.683 0 3.418.598 5.018 1.73q.367.26.72.553l.386.348c2.95 2.744 3.873 5.42 3.642 8.189c-.151 1.818-1.31 3.27-2.97 4.394c-1.58 1.07-4 1.377-5.727 1.192c-1.697-.182-3.456-.866-4.592-2.404c-.939-1.273-1.218-2.64-1.091-4.107c.127-1.459.712-2.823 1.662-3.772c.533-.533 1.442-1.202 2.894-1.324c-.68.156-1.33.48-1.887.976a4.29 4.29 0 0 0-1.378 3.869c.093.636.33 1.248.713 1.778a4.3 4.3 0 0 0 1.252 1.191c1.66 1.121 3.728 1.033 5.747-.306c1.1-.73 1.844-1.994 2.04-3.471c.238-1.788-.336-3.623-1.575-5.033c-1.347-1.533-3.212-2.44-5.116-2.49c-1.77-.046-3.409.652-4.737 2.017c-.407.417-.777.87-1.107 1.349q.358-.801.838-1.523C6.48 4.272 8.35 3.152 10.394 3.152z"/></svg>`,
      istio: `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 128 128"><path fill="#516baa" d="M58.802.013a.13.13 0 0 0-.073.11V101.73c0 .064.05.119.113.126l47.918 5.349a.13.13 0 0 0 .144-.114a.14.14 0 0 0-.014-.067L58.972.073a.127.127 0 0 0-.17-.06m-5.535 35.55a.127.127 0 0 0-.17.06l-31.99 71.4a.13.13 0 0 0-.01.07c.01.07.077.116.147.106l31.99-5.482a.12.12 0 0 0 .103-.123l.004-65.918a.13.13 0 0 0-.074-.114zM21.321 111.86a.12.12 0 0 0-.12.073a.126.126 0 0 0 .063.167l32.03 15.892c.03.01.064.01.093 0l53.006-15.892a.13.13 0 0 0 .07-.093a.126.126 0 0 0-.103-.147z"/></svg>`,
      'istio-gateway': `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><g fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="2"><path d="M21 17h-5.397a5 5 0 0 1-4.096-2.133l-.514-.734A5 5 0 0 0 6.897 12H3m18-5h-5.395a5 5 0 0 0-4.098 2.135l-.51.73A5 5 0 0 1 6.9 12H3"/><path d="m18 10l3-3l-3-3m0 16l3-3l-3-3"/></g></svg>`,
      keycloak: `<svg viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
        <rect x="4" y="6" width="16" height="12" rx="2" fill="#4D4D4D"/>
        <circle cx="12" cy="12" r="3" fill="#00D4AA"/>
        <path d="M12 9v6M9 12h6" stroke="#fff" stroke-width="1"/>
      </svg>`,
      loki: `<svg viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
        <rect x="3" y="3" width="18" height="18" rx="2" fill="#F46800"/>
        <path d="M7 7h10M7 12h10M7 17h6" stroke="#fff" stroke-width="2"/>
      </svg>`,
      mimir: `<svg viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
        <circle cx="12" cy="12" r="10" fill="#E6522C"/>
        <path d="M8 10l4 2 4-2M8 14l4 2 4-2" stroke="#fff" stroke-width="2"/>
      </svg>`,
      pinniped: `<svg viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
        <circle cx="12" cy="12" r="10" fill="#0F4C75"/>
        <path d="M8 10h8v4H8z" fill="#fff"/>
        <circle cx="10" cy="12" r="1" fill="#0F4C75"/>
        <circle cx="14" cy="12" r="1" fill="#0F4C75"/>
      </svg>`,
      'sealed-secrets': `<svg viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
        <rect x="4" y="8" width="16" height="12" rx="2" fill="#2E7D32"/>
        <path d="M8 8V6a4 4 0 0 1 8 0v2" stroke="#2E7D32" stroke-width="2" fill="none"/>
        <circle cx="12" cy="14" r="2" fill="#fff"/>
      </svg>`,
      tempo: `<svg viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
        <circle cx="12" cy="12" r="10" fill="#F46800"/>
        <path d="M12 6v6l4 2" stroke="#fff" stroke-width="2"/>
        <circle cx="12" cy="12" r="1" fill="#fff"/>
      </svg>`,
    };

    const svgContent = logos[logoName] || logos['postgresql'];
    return this.sanitizer.bypassSecurityTrustHtml(svgContent);
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
