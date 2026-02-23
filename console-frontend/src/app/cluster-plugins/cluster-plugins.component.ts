import { Component, inject, signal, OnInit, ChangeDetectionStrategy } from '@angular/core';
import { Router, ActivatedRoute } from '@angular/router';
import { NgIcon, provideIcons } from '@ng-icons/core';
import { tablerCircleXFill } from '@ng-icons/tabler-icons/fill';
import { create } from '@bufbuild/protobuf';
import { firstValueFrom } from 'rxjs';
import { TitleService } from '../title.service';
import { SharedPluginsFormComponent } from '../shared-plugins-form/shared-plugins-form.component';
import { CLUSTER } from '../../connect/tokens';
import {
  ListInstallsRequestSchema,
  AddInstallRequestSchema,
  RemoveInstallRequestSchema,
} from '../../generated/v1/cluster_pb';
import { fetchClusterName } from '../utils/cluster-status';

@Component({
  selector: 'app-cluster-plugins',
  imports: [SharedPluginsFormComponent, NgIcon],
  viewProviders: [
    provideIcons({
      tablerCircleXFill,
    }),
  ],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './cluster-plugins.component.html',
})
export default class ClusterPluginsComponent implements OnInit {
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
    await fetchClusterName(this.client, this.clusterId).then((name) => this.clusterName.set(name));
    try {
      // Fetch current installs for the cluster
      const listRequest = create(ListInstallsRequestSchema, {
        clusterId: this.clusterId,
      });
      const listResponse = await firstValueFrom(this.client.listInstalls(listRequest));
      this.currentPluginIds.set(listResponse.installs.map((install) => install.pluginId));
    } catch (error) {
      this.errorMessage.set(
        error instanceof Error
          ? `Failed to load current plugins: ${error.message}`
          : 'Failed to load current plugins',
      );
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
      await Promise.all(
        listResponse.installs
          .filter((install) => !data.plugins.includes(install.pluginId))
          .map((install) => {
            const removeRequest = create(RemoveInstallRequestSchema, {
              installId: install.id,
            });
            return firstValueFrom(this.client.removeInstall(removeRequest));
          }),
      );

      // Add plugins that are newly selected
      await Promise.all(
        data.plugins
          .filter((pluginId) => !currentPluginIds.includes(pluginId))
          .map((pluginId) => {
            const addRequest = create(AddInstallRequestSchema, {
              clusterId: this.clusterId,
              pluginId,
            });
            return firstValueFrom(this.client.addInstall(addRequest));
          }),
      );

      // Navigate back to cluster detail
      this.router.navigate(['/clusters', this.clusterId]);
    } catch (error) {
      const message =
        error instanceof Error
          ? `Failed to update cluster plugins: ${error.message}`
          : 'Failed to update cluster plugins';
      this.errorMessage.set(message);
    } finally {
      this.isSubmitting.set(false);
    }
  }

  onCancel() {
    this.router.navigate(['/clusters', this.clusterId]);
  }
}
