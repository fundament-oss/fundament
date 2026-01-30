import { Component, inject, signal, OnInit } from '@angular/core';
import { CommonModule } from '@angular/common';
import { Router, ActivatedRoute } from '@angular/router';
import { TitleService } from '../title.service';
import { SharedPluginsFormComponent } from '../shared-plugins-form/shared-plugins-form.component';
import { NgIcon, provideIcons } from '@ng-icons/core';
import { tablerCircleXFill } from '@ng-icons/tabler-icons/fill';
import { CLUSTER } from '../../connect/tokens';
import { create } from '@bufbuild/protobuf';
import {
  ListInstallsRequestSchema,
  AddInstallRequestSchema,
  RemoveInstallRequestSchema,
  GetClusterRequestSchema,
} from '../../generated/v1/cluster_pb';
import { firstValueFrom } from 'rxjs';
import { BreadcrumbComponent, BreadcrumbSegment } from '../breadcrumb/breadcrumb.component';

@Component({
  selector: 'app-cluster-plugins',
  standalone: true,
  imports: [CommonModule, SharedPluginsFormComponent, NgIcon, BreadcrumbComponent],
  viewProviders: [
    provideIcons({
      tablerCircleXFill,
    }),
  ],
  templateUrl: './cluster-plugins.component.html',
})
export class ClusterPluginsComponent implements OnInit {
  private titleService = inject(TitleService);
  private router = inject(Router);
  private route = inject(ActivatedRoute);
  private client = inject(CLUSTER);

  private clusterId = '';
  errorMessage = signal<string | null>(null);
  isSubmitting = signal(false);
  currentPluginIds = signal<string[]>([]);
  clusterName = signal<string | null>(null);

  constructor() {
    this.titleService.setTitle('Cluster plugins');
    this.clusterId = this.route.snapshot.paramMap.get('id') || '';
  }

  async ngOnInit() {
    await this.loadClusterName();
    try {
      // Fetch current installs for the cluster
      const listRequest = create(ListInstallsRequestSchema, {
        clusterId: this.clusterId,
      });
      const listResponse = await firstValueFrom(this.client.listInstalls(listRequest));
      this.currentPluginIds.set(listResponse.installs.map((install) => install.pluginId));
    } catch (error) {
      console.error('Failed to load current plugins:', error);
      this.errorMessage.set(
        error instanceof Error ? error.message : 'Failed to load current plugins',
      );
    }
  }

  async loadClusterName() {
    try {
      const request = create(GetClusterRequestSchema, { clusterId: this.clusterId });
      const response = await firstValueFrom(this.client.getCluster(request));
      if (response.cluster) {
        this.clusterName.set(response.cluster.name);
      }
    } catch (error) {
      console.error('Failed to load cluster name:', error);
    }
  }

  async onFormSubmit(data: { preset: string; plugins: string[] }) {
    // Note: data.plugins now contains UUIDs from the shared-plugins-form component
    if (this.isSubmitting()) return;

    this.errorMessage.set(null);
    this.isSubmitting.set(true);

    try {
      // Get current installs for the cluster
      const listRequest = create(ListInstallsRequestSchema, {
        clusterId: this.clusterId,
      });
      const listResponse = await firstValueFrom(this.client.listInstalls(listRequest));
      const currentPluginIds = listResponse.installs.map((install) => install.pluginId);

      // Remove plugins that are no longer selected
      for (const installId of listResponse.installs.map((install) => install.id)) {
        const pluginId = listResponse.installs.find(
          (install) => install.id === installId,
        )?.pluginId;
        if (pluginId && !data.plugins.includes(pluginId)) {
          const removeRequest = create(RemoveInstallRequestSchema, {
            installId: installId,
          });
          await firstValueFrom(this.client.removeInstall(removeRequest));
        }
      }

      // Add plugins that are newly selected
      for (const pluginId of data.plugins) {
        if (!currentPluginIds.includes(pluginId)) {
          const addRequest = create(AddInstallRequestSchema, {
            clusterId: this.clusterId,
            pluginId: pluginId,
          });
          await firstValueFrom(this.client.addInstall(addRequest));
        }
      }

      // Navigate back to cluster detail
      this.router.navigate(['/clusters', this.clusterId]);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Failed to update cluster plugins';
      this.errorMessage.set(message);
    } finally {
      this.isSubmitting.set(false);
    }
  }

  onCancel() {
    this.router.navigate(['/clusters', this.clusterId]);
  }

  get breadcrumbSegments(): BreadcrumbSegment[] {
    const segments: BreadcrumbSegment[] = [{ label: 'Clusters', route: '/' }];

    if (this.clusterName()) {
      segments.push({
        label: this.clusterName()!,
        route: `/clusters/${this.clusterId}`,
      });
    }

    segments.push({ label: 'Plugins' });

    return segments;
  }
}
