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

  /** False until the first answer that arrives, so an empty list is only
   *  reported as empty once it is known to be one. A failed read leaves it
   *  false: the estate is unknown, not empty. */
  readonly loaded = signal(false);

  /** What went wrong on the last read, so the menu can say so instead of
   *  standing there empty. Cleared by the next read that works. */
  readonly loadError = signal<string | null>(null);

  private pending = false;

  private queued = false;

  /** Reads the list again while keeping the one it has on screen, so a page
   *  that opens with this menu shows it filled and updates underneath. */
  load(): void {
    if (this.pending) {
      // A read asked for while one is in flight cannot be dropped: the answer
      // on the wire was sent before whatever prompted this one, so it does not
      // contain it. Remember it and read once more when this one lands.
      this.queued = true;
      return;
    }
    this.pending = true;
    firstValueFrom(this.dcApi.listSites())
      .then((res) => {
        this.datacenters.set(res.sites.map((s) => DatacenterApiService.mapSite(s)));
        this.loadError.set(null);
        this.loaded.set(true);
      })
      .catch((err) => {
        const message = connectErrorMessage(err);
        // eslint-disable-next-line no-console
        console.error(message);
        this.loadError.set(message);
      })
      .finally(() => {
        this.pending = false;
        if (this.queued) {
          this.queued = false;
          this.load();
        }
      });
  }
}
