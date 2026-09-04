import { computed, inject, Injectable, signal } from '@angular/core';
import { firstValueFrom } from 'rxjs';
import RackApiService from './rack-api.service';
import { Rack, RackDevice } from './rack.model';
import connectErrorMessage from '../../connect/error';

/** A rack as the menu lists it: the rack plus how full it is. */
export interface RackListItem extends Rack {
  usedU: number;
  freeU: number;
  totalPowerW: number;
  deviceCount: number;
  rowId: string;
}

/**
 * The racks of one data center, kept where a page change cannot take them.
 *
 * Two pages show the same menu: the rack itself and the page of one device in
 * it. Held in the page, the menu vanished the moment you opened a device, and
 * which data center you were in went with it.
 */
@Injectable({ providedIn: 'root' })
export default class RackListService {
  private readonly rackApi = inject(RackApiService);

  /** Which data center the section is in. The menu sets it, pages read it. */
  readonly selectedDcId = signal('');

  /** Writable on purpose: creating, renaming and deleting a rack write here. */
  readonly racks = signal<RackListItem[]>([]);

  /** The rack on screen when the address cannot say which one it is: the page
   *  of a device names the placement, not the rack it stands in. */
  readonly openRackId = signal('');

  /** False until the first answer, so an empty list is only empty once known. */
  readonly loaded = signal(false);

  /** Per building, not one flag for the service: reading every location at
   *  once would otherwise fetch the first and drop the rest. */
  private readonly pending = new Set<string>();

  /**
   * Reads the racks of one data center, keeping the ones on screen meanwhile.
   *
   * Only that building's rows are replaced, because the menu can be showing
   * every location at once and a read of one hall says nothing about another.
   */
  load(dcId: string): void {
    if (!dcId || this.pending.has(dcId)) return;
    this.pending.add(dcId);
    firstValueFrom(this.rackApi.listRacksBySite(dcId))
      .then((res) =>
        this.mergeSite(
          dcId,
          res.racks.flatMap((summary): RackListItem[] => {
            const rack = summary.rack;
            if (!rack) return [];
            return [
              {
                id: rack.id,
                name: rack.name,
                dcId,
                rowId: rack.rowId,
                totalU: rack.totalUnits,
                devices: [] as RackDevice[],
                usedU: summary.usedUnits,
                freeU: summary.freeUnits,
                totalPowerW: summary.powerDrawW,
                deviceCount: summary.deviceCount,
              },
            ];
          }),
        ),
      )
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)))
      .finally(() => {
        this.pending.delete(dcId);
        this.loaded.set(true);
      });
  }

  /** Every location at once, for the menu's All. */
  loadAll(dcIds: string[]): void {
    dcIds.forEach((dcId) => this.load(dcId));
  }

  private mergeSite(dcId: string, racks: RackListItem[]): void {
    this.racks.update((current) => [...current.filter((rack) => rack.dcId !== dcId), ...racks]);
  }

  /** What the menu shows: one building, or all of them in the order the data
   *  centers are listed. */
  readonly visible = computed(() => {
    const dcId = this.selectedDcId();
    const racks = dcId ? this.racks().filter((rack) => rack.dcId === dcId) : this.racks();
    return [...racks].sort((a, b) => a.name.localeCompare(b.name));
  });
}
