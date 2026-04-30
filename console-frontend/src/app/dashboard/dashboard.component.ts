import {
  Component,
  inject,
  signal,
  OnInit,
  OnDestroy,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
} from '@angular/core';
import { RouterLink } from '@angular/router';
import { firstValueFrom } from 'rxjs';
import { TitleService } from '../title.service';
import { ToastService } from '../toast.service';
import { OrganizationDataService } from '../organization-data.service';
import { CLUSTER } from '../../connect/tokens';
import { type ListClustersResponse_ClusterSummary as ClusterSummary } from '../../generated/v1/cluster_pb';
import { ClusterStatus } from '../../generated/v1/common_pb';
import { getStatusColor, getStatusLabel, isTransitionalStatus } from '../utils/cluster-status';

@Component({
  selector: 'app-dashboard',
  imports: [RouterLink],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './dashboard.component.html',
})
export default class DashboardComponent implements OnInit, OnDestroy {
  private titleService = inject(TitleService);

  private toastService = inject(ToastService);

  private organizationDataService = inject(OrganizationDataService);

  private client = inject(CLUSTER);

  private pollingTimer: ReturnType<typeof setInterval> | null = null;

  clusters = signal<ClusterSummary[]>([]);

  errorMessage = signal<string>('');

  // Expose utility functions for template
  getStatusColor = getStatusColor;

  getStatusLabel = getStatusLabel;

  constructor() {
    this.titleService.setTitle('Dashboard');
  }

  ngOnDestroy() {
    this.stopPolling();
  }

  async ngOnInit() {
    // Use cluster data pre-fetched during org initialization to avoid a duplicate
    // ListClusters call immediately after the one made by OrganizationDataService.
    const preloaded = this.organizationDataService.clusterSummaries();
    if (preloaded.length > 0 || this.organizationDataService.organizations().length > 0) {
      this.clusters.set(preloaded);
      if (preloaded.some((c) => isTransitionalStatus(c.status))) {
        this.pollingTimer = setInterval(() => this.loadClusters(), 5000);
      }
    } else {
      await this.loadClusters();
    }
  }

  private async loadClusters() {
    try {
      const response = await firstValueFrom(this.client.listClusters({}));
      const previousClusters = this.clusters();
      this.clusters.set(response.clusters);

      // Check if any previously-DELETING cluster has disappeared
      previousClusters
        .filter(
          (prev) =>
            prev.status === ClusterStatus.DELETING &&
            !response.clusters.some((c) => c.id === prev.id),
        )
        .forEach((prev) => {
          this.toastService.info(`Cluster '${prev.name}' has been deleted`);
        });

      const needsPolling = response.clusters.some((c) => isTransitionalStatus(c.status));
      if (needsPolling && !this.pollingTimer) {
        this.pollingTimer = setInterval(() => this.loadClusters(), 5000);
      } else if (!needsPolling) {
        this.stopPolling();
      }
    } catch (error) {
      this.errorMessage.set(
        error instanceof Error
          ? `Failed to load clusters: ${error.message}`
          : 'Failed to load clusters. Please try again later.',
      );
    }
  }

  private stopPolling() {
    if (this.pollingTimer) {
      clearInterval(this.pollingTimer);
      this.pollingTimer = null;
    }
  }
}
