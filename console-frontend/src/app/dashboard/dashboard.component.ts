import { Component, inject, signal, OnInit, CUSTOM_ELEMENTS_SCHEMA } from '@angular/core';
import { CommonModule } from '@angular/common';
import { RouterLink } from '@angular/router';
import { TitleService } from '../title.service';
import { CLUSTER } from '../../connect/tokens';
import { ClusterSummary } from '../../generated/v1/cluster_pb';
import { firstValueFrom } from 'rxjs';
import { getStatusColor, getStatusLabel } from '../utils/cluster-status';

@Component({
  selector: 'app-dashboard',
  standalone: true,
  imports: [CommonModule, RouterLink],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  templateUrl: './dashboard.component.html',
})
export class DashboardComponent implements OnInit {
  private titleService = inject(TitleService);
  private client = inject(CLUSTER);

  clusters = signal<ClusterSummary[]>([]);
  errorMessage = signal<string>('');
  nodePoolCounts = signal<Map<string, number>>(new Map());

  // Expose utility functions for template
  getStatusColor = getStatusColor;
  getStatusLabel = getStatusLabel;

  constructor() {
    this.titleService.setTitle('Dashboard');
  }

  async ngOnInit() {
    try {
      const response = await firstValueFrom(this.client.listClusters({}));
      this.clusters.set(response.clusters);

      // Fetch node pools for each cluster
      for (const cluster of response.clusters) {
        try {
          const poolsResponse = await firstValueFrom(
            this.client.listNodePools({ clusterId: cluster.id }),
          );
          const counts = new Map(this.nodePoolCounts());
          counts.set(cluster.id, poolsResponse.nodePools.length);
          this.nodePoolCounts.set(counts);
        } catch (error) {
          console.error(`Failed to load node pools for cluster ${cluster.id}:`, error);
        }
      }
    } catch (error) {
      console.error('Failed to load clusters:', error);
      this.errorMessage.set(
        error instanceof Error ? error.message : 'Failed to load clusters. Please try again later.',
      );
    }
  }

  getNodePoolCount(clusterId: string): number {
    return this.nodePoolCounts().get(clusterId) || 0;
  }
}
