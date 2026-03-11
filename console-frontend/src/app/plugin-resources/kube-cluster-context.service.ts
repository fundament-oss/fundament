import { Injectable, signal, inject } from '@angular/core';
import { create } from '@bufbuild/protobuf';
import { firstValueFrom } from 'rxjs';
import { CLUSTER } from '../../connect/tokens';
import { ListClustersRequestSchema } from '../../generated/v1/cluster_pb';
import type { ListClustersResponse_ClusterSummary as ClusterSummary } from '../../generated/v1/cluster_pb';

@Injectable({ providedIn: 'root' })
export default class KubeClusterContextService {
  private clusterClient = inject(CLUSTER);

  clusters = signal<ClusterSummary[]>([]);

  selectedClusterId = signal<string>('');

  isLoadingClusters = signal(true);

  private loadStarted = false;

  async loadClusters(): Promise<void> {
    if (this.loadStarted) return;
    this.loadStarted = true;
    try {
      const response = await firstValueFrom(
        this.clusterClient.listClusters(create(ListClustersRequestSchema, {})),
      );
      this.clusters.set(response.clusters);
      if (response.clusters.length > 0) {
        this.selectedClusterId.set(response.clusters[0].id);
      }
    } finally {
      this.isLoadingClusters.set(false);
    }
  }

  onClusterChange(clusterId: string): void {
    this.selectedClusterId.set(clusterId);
  }
}
