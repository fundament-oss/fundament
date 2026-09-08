import { computed, inject, Injectable, signal } from '@angular/core';
import { firstValueFrom } from 'rxjs';
import DatacenterApiService from './datacenter-api.service';
import { DatacenterStatus, statusTagColor } from './datacenter.model';
import connectErrorMessage from '../../connect/error';

/**
 * How the data centers are doing, as one state: the worst one of them all.
 *
 * The section list shows it as a dot beside "Data centers", so the shell says
 * something is wrong before you go looking. Held in a service rather than in
 * the page, because the shell outlives every page and the page that owns the
 * data is not open most of the time.
 */
@Injectable({ providedIn: 'root' })
export default class DatacenterHealthService {
  private readonly datacenterApi = inject(DatacenterApiService);

  private readonly statuses = signal<DatacenterStatus[]>([]);

  private pending = false;

  /**
   * The state that needs attention first. Degraded outranks maintenance:
   * something broken is worse news than something planned.
   */
  readonly worst = computed<DatacenterStatus | null>(() => {
    const all = this.statuses();
    if (all.length === 0) return null;
    if (all.includes('degraded')) return 'degraded';
    if (all.includes('maintenance')) return 'maintenance';
    return 'operational';
  });

  /** The badge color for that state, the same one the rows use. */
  readonly worstColor = computed<string | null>(() => {
    const worst = this.worst();
    return worst ? statusTagColor(worst) : null;
  });

  refresh(): void {
    if (this.pending) return;
    this.pending = true;
    firstValueFrom(this.datacenterApi.listSites())
      .then((res) =>
        this.statuses.set(res.sites.map((site) => DatacenterApiService.mapSite(site).status)),
      )
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)))
      .finally(() => {
        this.pending = false;
      });
  }
}
