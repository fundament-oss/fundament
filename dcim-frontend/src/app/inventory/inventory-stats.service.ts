import { inject, Injectable, signal } from '@angular/core';
import { firstValueFrom } from 'rxjs';
import type { AssetStats } from '../../generated/v1/asset_pb';
import InventoryApiService from './inventory-api.service';
import connectErrorMessage from '../../connect/error';

/**
 * The counts per status, kept where a page change cannot take them with it.
 *
 * Three places show them: the menu of the section, the same menu on the page of
 * one asset, and the badge on Inventory in the section list. Each of those used
 * to fetch its own copy, so walking from the list into an asset tore the menu
 * down, built it again and left the numbers blank until the answer came back —
 * a flicker on every click. Held here they are already there, and the refresh
 * happens underneath.
 */
@Injectable({ providedIn: 'root' })
export default class InventoryStatsService {
  private readonly inventoryApi = inject(InventoryApiService);

  private readonly value = signal<AssetStats | null>(null);

  /** What was last loaded, or null before the first answer. */
  readonly stats = this.value.asReadonly();

  private pending = false;

  /** Loads once and keeps the last answer visible while a new one is on its way. */
  /** Bumped when an asset is made or changed somewhere else than the page
   *  showing the list, so that page can read it again. */
  readonly changed = signal(0);

  /** Say that an asset changed: the counts follow and so does the list. */
  markChanged(): void {
    this.changed.update((n) => n + 1);
    this.refresh();
  }

  refresh(): void {
    if (this.pending) return;
    this.pending = true;
    firstValueFrom(this.inventoryApi.getAssetStats())
      .then((res) => this.value.set(res.stats ?? null))
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)))
      .finally(() => {
        this.pending = false;
      });
  }
}
