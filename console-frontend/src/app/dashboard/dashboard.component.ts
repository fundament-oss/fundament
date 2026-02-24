import {
  Component,
  inject,
  signal,
  OnInit,
  OnDestroy,
  ChangeDetectionStrategy,
} from '@angular/core';
import { RouterLink } from '@angular/router';
import { firstValueFrom } from 'rxjs';
import { NgIcon, provideIcons } from '@ng-icons/core';
import { tablerPlus, tablerEye } from '@ng-icons/tabler-icons';
import { tablerCircleXFill } from '@ng-icons/tabler-icons/fill';
import { TitleService } from '../title.service';
import { ToastService } from '../toast.service';
import { CLUSTER } from '../../connect/tokens';
import { type ListClustersResponse_ClusterSummary as ClusterSummary } from '../../generated/v1/cluster_pb';
import { ClusterStatus } from '../../generated/v1/common_pb';
import { getStatusColor, getStatusLabel, isTransitionalStatus } from '../utils/cluster-status';

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
export default class DashboardComponent implements OnInit, OnDestroy {
  private titleService = inject(TitleService);

  private toastService = inject(ToastService);

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
    await this.loadClusters();
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
