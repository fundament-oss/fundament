import { Component, inject, signal, OnInit } from '@angular/core';
import { CommonModule } from '@angular/common';
import { Router, ActivatedRoute } from '@angular/router';
import { TitleService } from '../title.service';
import { SharedNodePoolsFormComponent, NodePoolData } from '../shared-node-pools-form/shared-node-pools-form.component';
import { CLUSTER } from '../../connect/tokens';
import { create } from '@bufbuild/protobuf';
import {
  ListNodePoolsRequestSchema,
  CreateNodePoolRequestSchema,
  UpdateNodePoolRequestSchema,
  DeleteNodePoolRequestSchema,
  NodePool
} from '../../generated/v1/cluster_pb';
import { firstValueFrom } from 'rxjs';
import { ErrorIconComponent } from '../icons';

@Component({
  selector: 'app-cluster-nodes',
  standalone: true,
  imports: [CommonModule, SharedNodePoolsFormComponent, ErrorIconComponent],
  templateUrl: './cluster-nodes.component.html',
})
export class ClusterNodesComponent implements OnInit {
  private titleService = inject(TitleService);
  private router = inject(Router);
  private route = inject(ActivatedRoute);
  private client = inject(CLUSTER);

  private clusterId = '';
  private existingNodePools: NodePool[] = [];
  errorMessage = signal<string | null>(null);
  isSubmitting = signal(false);
  isLoading = signal(true);
  initialNodePools = signal<NodePoolData[]>([]);

  constructor() {
    this.titleService.setTitle('Cluster nodes');
    this.clusterId = this.route.snapshot.paramMap.get('id') || '';
  }

  async ngOnInit() {
    await this.loadNodePools();
  }

  async loadNodePools() {
    try {
      this.isLoading.set(true);
      const request = create(ListNodePoolsRequestSchema, {
        clusterId: this.clusterId,
      });
      const response = await firstValueFrom(this.client.listNodePools(request));
      this.existingNodePools = response.nodePools;
      
      // Convert to NodePoolData format for the form
      this.initialNodePools.set(
        response.nodePools.map(pool => ({
          name: pool.name,
          machineType: pool.machineType,
          autoscaleMin: pool.minNodes,
          autoscaleMax: pool.maxNodes,
        }))
      );
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Failed to load node pools';
      this.errorMessage.set(message);
    } finally {
      this.isLoading.set(false);
    }
  }

  async onFormSubmit(data: { nodePools: NodePoolData[] }) {
    if (this.isSubmitting()) return;

    this.errorMessage.set(null);
    this.isSubmitting.set(true);

    try {
      const newPools = data.nodePools;
      const existingPoolsMap = new Map(this.existingNodePools.map(p => [p.name, p]));
      const newPoolsMap = new Map(newPools.map(p => [p.name, p]));

      // Delete pools that no longer exist
      for (const existingPool of this.existingNodePools) {
        if (!newPoolsMap.has(existingPool.name)) {
          const deleteRequest = create(DeleteNodePoolRequestSchema, {
            nodePoolId: existingPool.id,
          });
          await firstValueFrom(this.client.deleteNodePool(deleteRequest));
        }
      }

      // Create or update pools
      for (const newPool of newPools) {
        const existingPool = existingPoolsMap.get(newPool.name);
        
        if (existingPool) {
          // Update if values changed
          if (
            existingPool.minNodes !== newPool.autoscaleMin ||
            existingPool.maxNodes !== newPool.autoscaleMax
          ) {
            const updateRequest = create(UpdateNodePoolRequestSchema, {
              nodePoolId: existingPool.id,
              autoscaleMin: newPool.autoscaleMin,
              autoscaleMax: newPool.autoscaleMax,
            });
            await firstValueFrom(this.client.updateNodePool(updateRequest));
          }
        } else {
          // Create new pool
          const createRequest = create(CreateNodePoolRequestSchema, {
            clusterId: this.clusterId,
            name: newPool.name,
            machineType: newPool.machineType,
            autoscaleMin: newPool.autoscaleMin,
            autoscaleMax: newPool.autoscaleMax,
          });
          await firstValueFrom(this.client.createNodePool(createRequest));
        }
      }

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
