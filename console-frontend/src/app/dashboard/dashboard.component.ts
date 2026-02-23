import { Component, inject, signal, OnInit, ChangeDetectionStrategy } from '@angular/core';
import { RouterLink } from '@angular/router';
import { firstValueFrom } from 'rxjs';
import { NgIcon, provideIcons } from '@ng-icons/core';
import { tablerPlus, tablerEye } from '@ng-icons/tabler-icons';
import { tablerCircleXFill } from '@ng-icons/tabler-icons/fill';
import { TitleService } from '../title.service';
import { CLUSTER } from '../../connect/tokens';
import { type ListClustersResponse_ClusterSummary as ClusterSummary } from '../../generated/v1/cluster_pb';
import { getStatusColor, getStatusLabel } from '../utils/cluster-status';

@Component({
  selector: 'app-dashboard',
  imports: [RouterLink, NgIcon],
  viewProviders: [
    provideIcons({
      tablerCircleXFill,
      tablerPlus,
      tablerEye,
    }),
  ],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './dashboard.component.html',
})
export default class DashboardComponent implements OnInit {
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
      const poolResults = await Promise.all(
        response.clusters.map((cluster) =>
          firstValueFrom(this.client.listNodePools({ clusterId: cluster.id }))
            .then((poolsResponse) => ({
              clusterId: cluster.id,
              count: poolsResponse.nodePools.length,
            }))
            .catch((error) => {
              this.errorMessage.set(
                error instanceof Error
                  ? `Failed to load node pools for cluster ${cluster.id}: ${error.message}`
                  : 'Failed to load node pools for cluster.',
              );
              return null;
            }),
        ),
      );
      const counts = new Map<string, number>();
      poolResults.forEach((result) => {
        if (result) {
          counts.set(result.clusterId, result.count);
        }
      });
      this.nodePoolCounts.set(counts);
    } catch (error) {
      this.errorMessage.set(
        error instanceof Error
          ? `Failed to load clusters: ${error.message}`
          : 'Failed to load clusters. Please try again later.',
      );
    }
  }

  getNodePoolCount(clusterId: string): number {
    return this.nodePoolCounts().get(clusterId) || 0;
  }
}
