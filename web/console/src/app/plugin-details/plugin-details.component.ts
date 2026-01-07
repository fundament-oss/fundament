import { Component, inject } from '@angular/core';
import { CommonModule } from '@angular/common';
import { DomSanitizer, SafeHtml } from '@angular/platform-browser';
import { RouterLink } from '@angular/router';
import { TitleService } from '../title.service';
import { InstallPluginModalComponent } from '../install-plugin-modal/install-plugin-modal';
import {
  ChevronRightIconComponent,
  CheckmarkIconComponent,
  ExternalLinkIconComponent,
} from '../icons';

@Component({
  selector: 'app-plugin-details',
  standalone: true,
  imports: [
    CommonModule,
    RouterLink,
    InstallPluginModalComponent,
    ChevronRightIconComponent,
    CheckmarkIconComponent,
    ExternalLinkIconComponent,
  ],
  templateUrl: './plugin-details.component.html',
})
export class PluginDetailsComponent {
  private titleService = inject(TitleService);
  private sanitizer = inject(DomSanitizer);

  pluginId = 'grafana';
  pluginName = 'Grafana';

  installedClusters = ['cluster-1', 'cluster-4'];

  clusters = [
    { id: 'cluster-1', name: 'cluster-1', installed: true },
    { id: 'cluster-2', name: 'cluster-2', installed: false },
    { id: 'cluster-3', name: 'cluster-3', installed: false },
    { id: 'cluster-4', name: 'cluster-4', installed: true },
  ];

  showInstallModal = false;

  pluginDescription = `
# Overview

Grafana is the open source analytics and monitoring solution for every database. It allows you to query, visualize, alert on and understand your metrics no matter where they are stored.

## Key Features

- **Visualization**: Create stunning dashboards with a variety of visualization options
- **Alerting**: Define alert rules and get notified when metrics exceed thresholds
- **Data Sources**: Connect to multiple data sources including Prometheus, InfluxDB, and more
- **Plugins**: Extend functionality with a rich ecosystem of plugins

## Use Cases

- Infrastructure monitoring
- Application performance monitoring
- Business analytics
- IoT data visualization
  `;

  pricing = {
    type: 'free',
    details: 'Open source and free to use',
  };

  author = {
    name: 'Grafana Labs',
    website: 'https://grafana.com',
  };

  documentation = {
    url: 'https://grafana.com/docs/',
    label: 'Official documentation',
  };

  constructor() {
    this.titleService.setTitle(`${this.pluginName} — Plugins`);
  }

  getRenderedMarkdown(): SafeHtml {
    // Simple markdown to HTML conversion
    let html = this.pluginDescription
      .replace(/^# (.*$)/gim, '<h1 class="text-2xl font-bold mb-3 dark:text-white">$1</h1>')
      .replace(
        /^## (.*$)/gim,
        '<h2 class="text-xl font-semibold mb-2 mt-4 dark:text-white">$1</h2>',
      )
      .replace(/^- (.*$)/gim, '<li class="ml-4">$1</li>')
      .replace(/\*\*(.*?)\*\*/g, '<strong class="font-semibold">$1</strong>')
      .replace(/\n\n/g, '</p><p class="mb-3">')
      .trim();

    html = '<p class="mb-3">' + html + '</p>';

    return this.sanitizer.sanitize(1, html) || '';
  }

  openInstallModal(): void {
    this.showInstallModal = true;
  }

  closeInstallModal(): void {
    this.showInstallModal = false;
  }

  onInstallOnCluster(clusterId: string): void {
    const cluster = this.clusters.find((c) => c.id === clusterId);
    if (cluster && !cluster.installed) {
      cluster.installed = true;
      this.installedClusters.push(clusterId);
    }
  }

  isInstalled(clusterId: string): boolean {
    return this.installedClusters.includes(clusterId);
  }
}
