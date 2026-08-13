import {
  ChangeDetectionStrategy,
  Component,
  computed,
  CUSTOM_ELEMENTS_SCHEMA,
  inject,
  OnInit,
} from '@angular/core';
import { toSignal } from '@angular/core/rxjs-interop';
import { NavigationEnd, Router } from '@angular/router';
import { filter, map, startWith } from 'rxjs';
import InventoryStatsService from './inventory-stats.service';
import { ASSET_STATUS_TAG_COLOR } from './asset-status';
import categoryIcon, { AssetCategory, CATEGORIES } from '../shared/asset-category';
import { viewSlug } from '../shared/section-views';
import { INVENTORY_PATH } from './inventory-views';
import type { AssetStatus } from './inventory';

/**
 * The menu of the inventory section, in a component of its own because two
 * pages show it: the list, and the page of one asset. Opening an asset replaces
 * the list beside the menu, not the menu itself — you are still in the
 * inventory, so the way to another view has to stay where it was.
 *
 * It reads the address rather than taking inputs: which row is current follows
 * from the URL, and both pages are at a different one.
 */
@Component({
  selector: 'app-inventory-nav',
  changeDetection: ChangeDetectionStrategy.OnPush,
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  templateUrl: './inventory-nav.html',
})
export default class InventoryNavComponent implements OnInit {
  private readonly statsService = inject(InventoryStatsService);

  private readonly router = inject(Router);

  /** The address as it is now, so a row lights up on any page that shows this menu. */
  private readonly url = toSignal(
    this.router.events.pipe(
      filter((e): e is NavigationEnd => e instanceof NavigationEnd),
      map((e) => e.urlAfterRedirects),
      startWith(this.router.url),
    ),
    { initialValue: this.router.url },
  );

  ngOnInit(): void {
    this.statsService.refresh();
  }

  readonly categories = CATEGORIES;

  readonly categoryIcon = categoryIcon;

  readonly statusTagColor = (status: AssetStatus): string => ASSET_STATUS_TAG_COLOR[status];

  readonly statuses: { value: AssetStatus; label: string }[] = [
    { value: 'needs-repair', label: 'Needs Repair' },
    { value: 'requested', label: 'Requested' },
    { value: 'on-order', label: 'On Order' },
    { value: 'available', label: 'Available' },
    { value: 'deployed', label: 'Deployed' },
    { value: 'decommissioned', label: 'Decommissioned' },
  ];

  readonly statusCounts = computed<Record<string, number>>(() => {
    const s = this.statsService.stats();
    if (!s) return {};
    const counts: Record<string, number> = {
      all: s.total,
      deployed: s.deployed,
      available: s.available,
      'on-order': s.onOrder,
      requested: s.requested,
      'needs-repair': s.needsRepair,
      decommissioned: s.decommissioned,
    };
    return counts;
  });

  /** The address of a view, so every row is a real link. */
  readonly viewPath = (kind: 'all' | 'status' | 'category', value = ''): string =>
    kind === 'all' ? `${INVENTORY_PATH}/all` : `${INVENTORY_PATH}/${kind}/${viewSlug(value)}`;

  /**
   * Whether this row is the page you are on. Matched against the address, so an
   * asset page (which is no view) lights up nothing.
   */
  isCurrent(kind: 'all' | 'status' | 'category', value = ''): boolean {
    const path = this.url().split('?')[0];
    if (kind === 'all') return path === `${INVENTORY_PATH}/all`;
    return path === this.viewPath(kind, value);
  }

  /** A real link, routed in-app unless the click asks for a new tab or window. */
  goToView(event: Event, kind: 'all' | 'status' | 'category', value = ''): void {
    if (event instanceof MouseEvent) {
      if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey || event.button !== 0) {
        return;
      }
    }

    event.preventDefault();
    this.router.navigateByUrl(this.viewPath(kind, value));
  }
}

export type { AssetCategory };
