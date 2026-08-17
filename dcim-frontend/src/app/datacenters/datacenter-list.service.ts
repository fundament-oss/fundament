import { inject, Injectable, signal } from '@angular/core';
import { firstValueFrom } from 'rxjs';
import DatacenterApiService from './datacenter-api.service';
import { DatacenterInfo } from './datacenter.model';
import connectErrorMessage from '../../connect/error';

/**
 * Every data center, kept where a page change cannot take them with it.
 *
 * Three pages show the same menu: the list, one data center, and that data
 * center's layout. Each of them used to fetch its own copy, so every step tore
 * the menu down, built it again and showed an empty list until the answer came
 * back. Held here it is already there, and the pages write their changes into
 * the same signal so a rename or a new one shows up everywhere at once.
 */
@Injectable({ providedIn: 'root' })
export default class DatacenterListService {
  private readonly dcApi = inject(DatacenterApiService);

  /** Writable on purpose: the pages that create, rename and delete a data
   *  center update this list rather than fetching it again. */
  readonly datacenters = signal<DatacenterInfo[]>([]);

  /** False until the first answer, so an empty list is only reported as empty
   *  once it is known to be one. */
  readonly loaded = signal(false);

  private pending = false;

  /** Reads the list again while keeping the one it has on screen, so a page
   *  that opens with this menu shows it filled and updates underneath. */
  load(): void {
    if (this.pending) return;
    this.pending = true;
    firstValueFrom(this.dcApi.listSites())
      .then((res) => this.datacenters.set(res.sites.map((s) => DatacenterApiService.mapSite(s))))
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)))
      .finally(() => {
        this.pending = false;
        this.loaded.set(true);
      });
  }
}
