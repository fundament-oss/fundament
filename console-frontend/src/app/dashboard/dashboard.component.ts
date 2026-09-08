import {
  Component,
  inject,
  signal,
  OnInit,
  OnDestroy,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
} from '@angular/core';
import { RouterOutlet } from '@angular/router';
import { firstValueFrom } from 'rxjs';
import { TitleService } from '../title.service';
import { NotificationService } from '../notification.service';
import { OrganizationDataService } from '../organization-data.service';
import { CLUSTER } from '../../connect/tokens';
import { type ListClustersResponse_ClusterSummary as ClusterSummary } from '../../generated/v1/cluster_pb';
import { ClusterStatus } from '../../generated/v1/common_pb';
import { getStatusBadgeColor, getStatusLabel, isTransitionalStatus } from '../utils/cluster-status';
import PageNavService from '../page-nav.service';

import '@nldd/design-system/badge';
import '@nldd/design-system/banner';
import '@nldd/design-system/button';
import '@nldd/design-system/card';
import '@nldd/design-system/collection';
import '@nldd/design-system/container';
import '@nldd/design-system/inline-dialog';
import '@nldd/design-system/list';
import '@nldd/design-system/list-item';
import '@nldd/design-system/menu';
import '@nldd/design-system/page';
import '@nldd/design-system/rich-text';
import '@nldd/design-system/simple-section';
import '@nldd/design-system/spacer';
import '@nldd/design-system/spacer-cell';
import '@nldd/design-system/text-cell';
import '@nldd/design-system/title';
import '@nldd/design-system/toolbar';
import '@nldd/design-system/top-title-bar';

@Component({
  selector: 'app-dashboard',
  imports: [RouterOutlet],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './dashboard.component.html',
})
export default class DashboardComponent implements OnInit, OnDestroy {
  protected pageNav = inject(PageNavService);

  private titleService = inject(TitleService);

  private notificationService = inject(NotificationService);

  private organizationDataService = inject(OrganizationDataService);

  private client = inject(CLUSTER);

  private pollingTimer: ReturnType<typeof setInterval> | null = null;

  clusters = signal<ClusterSummary[]>([]);

  errorMessage = signal<string>('');

  // Expose utility functions for template
  getStatusBadgeColor = getStatusBadgeColor;

  isTransitionalStatus = isTransitionalStatus;

  getStatusLabel = getStatusLabel;

  constructor() {
    this.titleService.setTitle('Clusters');
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
          this.notificationService.success(`Cluster '${prev.name}' has been deleted`);
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
