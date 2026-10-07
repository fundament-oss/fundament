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
import sortByName from '../utils/sort-by-name';
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

  /** A request in flight outlives the page it was asked from, and its answer
   *  must not land on a page that is gone: clearing the timer is not enough,
   *  because the answer starts a new one. */
  private destroyed = false;

  ngOnDestroy() {
    this.destroyed = true;
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
    // Minted before the request: the timer asks again without waiting for the
    // answer, and a delete asks in between, so the answers do not necessarily
    // come back in the order they were asked for.
    const ticket = this.organizationDataService.beginClusterListFetch();

    let fetched: ClusterSummary[];
    try {
      const response = await firstValueFrom(this.client.listClusters({}));
      fetched = response.clusters;
    } catch (error) {
      this.errorMessage.set(
        error instanceof Error
          ? `Failed to load clusters: ${error.message}`
          : 'Failed to load clusters. Please try again later.',
      );
      return;
    }

    // The poll is the only thing watching a cluster that is being deleted, so
    // the shared cache hears about it from here: it holds the cluster until the
    // server stops listing it, and the sidebar counts what it holds.
    const applied = this.organizationDataService.setClusters(fetched, ticket);

    // The cache still wanted the list, but there is no page here any more to
    // show it, and starting a poll now would leave behind an interval that
    // ngOnDestroy has stopped looking for.
    if (this.destroyed) return;

    // A refused list was overtaken, so the cache holds the newer of the two and
    // that is what the page shows. It says nothing about what changed: the list
    // that won reported on itself when it landed, and a refusal can also mean
    // the organization was switched, whose clusters are nobody's news here.
    const clusters = applied
      ? sortByName(fetched)
      : this.organizationDataService.clusterSummaries();

    const previousClusters = this.clusters();
    this.clusters.set(clusters);

    // Check if any previously-DELETING cluster has disappeared
    if (applied) {
      previousClusters
        .filter(
          (prev) =>
            prev.status === ClusterStatus.DELETING && !clusters.some((c) => c.id === prev.id),
        )
        .forEach((prev) => {
          this.notificationService.success(`Cluster '${prev.name}' has been deleted`);
        });
    }

    const needsPolling = clusters.some((c) => isTransitionalStatus(c.status));
    if (needsPolling && !this.pollingTimer) {
      this.pollingTimer = setInterval(() => this.loadClusters(), 5000);
    } else if (!needsPolling) {
      this.stopPolling();
    }
  }

  private stopPolling() {
    if (this.pollingTimer) {
      clearInterval(this.pollingTimer);
      this.pollingTimer = null;
    }
  }
}
