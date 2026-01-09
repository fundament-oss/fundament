import { Component, inject, signal } from '@angular/core';
import { CommonModule } from '@angular/common';
import { Router, ActivatedRoute } from '@angular/router';
import { TitleService } from '../title.service';
import {
  SharedNodePoolsFormComponent,
  NodePoolData,
} from '../shared-node-pools-form/shared-node-pools-form.component';
import { CLUSTER } from '../../connect/tokens';
import { create } from '@bufbuild/protobuf';
import { UpdateClusterRequestSchema, NodePoolSpecSchema } from '../../generated/v1/cluster_pb';
import { firstValueFrom } from 'rxjs';
import { ErrorIconComponent } from '../icons';

@Component({
  selector: 'app-cluster-nodes',
  standalone: true,
  imports: [CommonModule, SharedNodePoolsFormComponent, ErrorIconComponent],
  templateUrl: './cluster-nodes.component.html',
})
export class ClusterNodesComponent {
  private titleService = inject(TitleService);
  private router = inject(Router);
  private route = inject(ActivatedRoute);
  private client = inject(CLUSTER);

  private clusterId = '';
  errorMessage = signal<string | null>(null);
  isSubmitting = signal(false);

  constructor() {
    this.titleService.setTitle('Cluster nodes');
    this.clusterId = this.route.snapshot.paramMap.get('id') || '';
  }

  async onFormSubmit(data: { nodePools: NodePoolData[] }) {
    if (this.isSubmitting()) return;

    this.errorMessage.set(null);
    this.isSubmitting.set(true);

    try {
      // Build the update request
      const request = create(UpdateClusterRequestSchema, {
        clusterId: this.clusterId,
        nodePools: data.nodePools.map((pool) =>
          create(NodePoolSpecSchema, {
            name: pool.name,
            machineType: pool.machineType,
            autoscaleMin: pool.autoscaleMin,
            autoscaleMax: pool.autoscaleMax,
          }),
        ),
      });

      await firstValueFrom(this.client.updateCluster(request));

      // Navigate back to cluster overview on success
      this.router.navigate(['/clusters', this.clusterId]);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Failed to update cluster nodes';
      this.errorMessage.set(message);
    } finally {
      this.isSubmitting.set(false);
    }
  }

  onCancel() {
    this.router.navigate(['/clusters', this.clusterId]);
  }
}
