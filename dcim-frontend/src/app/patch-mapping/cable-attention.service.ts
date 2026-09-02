import { computed, inject, Injectable, signal } from '@angular/core';
import { firstValueFrom } from 'rxjs';
import DatacenterApiService from '../datacenters/datacenter-api.service';
import PatchMappingApiService from './patch-mapping-api.service';
import connectErrorMessage from '../../connect/error';

/**
 * How many cables are waiting on somebody, over the whole estate.
 *
 * One number, for the badge on Patch mapping in the section list. It counts the
 * two states that ask something of a person: one still has to be bought, the
 * other is lying in a rack waiting to be fitted. Ordered is left out, because
 * there the wait is the supplier's and nobody here can shorten it.
 *
 * Counted by reading every site's connections, which is one call per data
 * center. Assets have a stats message on the server and this should have one
 * too; until then this is the honest cost of the number, and it is paid once at
 * start-up and again when a cable changes.
 */
@Injectable({ providedIn: 'root' })
export default class CableAttentionService {
  private readonly dcApi = inject(DatacenterApiService);

  private readonly patchApi = inject(PatchMappingApiService);

  private readonly value = signal(0);

  /** What was last counted, or 0 before the first answer. */
  readonly count = computed(() => this.value());

  private pending = false;

  private queued = false;

  refresh(): void {
    if (this.pending) {
      // A refresh asked for while one is in flight cannot be dropped: the
      // answer on the wire was sent before whatever prompted this one, so it
      // does not contain it. Remember it and count once more when this lands.
      this.queued = true;
      return;
    }
    this.pending = true;
    this.load()
      .catch((err) => {
        // eslint-disable-next-line no-console
        console.error(connectErrorMessage(err));
      })
      .finally(() => {
        this.pending = false;
        if (this.queued) {
          this.queued = false;
          this.refresh();
        }
      });
  }

  private async load(): Promise<void> {
    const sites = await firstValueFrom(this.dcApi.listSites());
    const perSite = await Promise.all(
      sites.sites.map((site) => firstValueFrom(this.patchApi.listConnectionsBySite(site.id))),
    );
    const waiting = perSite
      .flatMap((res) => res.connections)
      .filter((connection) => {
        const status = PatchMappingApiService.cableStatusFromProto(connection.status);
        return status === 'to-order' || status === 'ready-to-install';
      });
    this.value.set(waiting.length);
  }
}
