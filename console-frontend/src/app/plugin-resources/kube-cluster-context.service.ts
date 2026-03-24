import { Injectable, inject, signal } from '@angular/core';
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

  private loadPromise: Promise<void> | null = null;

  loadClusters(): Promise<void> {
    this.loadPromise ??= this.doLoad();
    return this.loadPromise;
  }

  private async doLoad(): Promise<void> {
    try {
      const response = await firstValueFrom(
        this.clusterClient.listClusters(create(ListClustersRequestSchema, {})),
      );
      this.clusters.set(response.clusters);
      if (response.clusters.length > 0) {
        this.selectedClusterId.set(response.clusters[0].id);
      }
    } catch (err) {
      // Reset so the next call can retry instead of re-using the rejected promise.
      this.loadPromise = null;
      throw err;
    } finally {
      this.isLoadingClusters.set(false);
    }
  }

  onClusterChange(clusterId: string): void {
    this.selectedClusterId.set(clusterId);
  }
}
