import {
  ChangeDetectionStrategy,
  Component,
  computed,
  CUSTOM_ELEMENTS_SCHEMA,
  inject,
  OnInit,
  signal,
} from '@angular/core';
import { toSignal } from '@angular/core/rxjs-interop';
import { NavigationEnd, Router } from '@angular/router';
import { filter, firstValueFrom, map, startWith } from 'rxjs';
import CatalogApiService from './catalog-api.service';
import connectErrorMessage from '../../connect/error';
import categoryIcon, { AssetCategory, CATEGORIES } from '../shared/asset-category';
import { viewSlug } from '../shared/section-views';
import { CATALOG_PATH } from './catalog-views';

/**
 * The menu of the catalog section, in a component of its own because two pages
 * show it: the list of products, and the page of one product. Opening a product
 * replaces the list beside the menu, not the menu itself.
 *
 * It reads the address rather than taking inputs: which row is current follows
 * from the URL, and both pages are at a different one.
 */
@Component({
  selector: 'app-catalog-nav',
  changeDetection: ChangeDetectionStrategy.OnPush,
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  templateUrl: './catalog-nav.html',
})
export default class CatalogNavComponent implements OnInit {
  private readonly catalogApi = inject(CatalogApiService);

  private readonly router = inject(Router);

  /** Every entry, for the count per category and the total. */
  private readonly entries = signal<{ category: AssetCategory }[]>([]);

  private readonly url = toSignal(
    this.router.events.pipe(
      filter((e): e is NavigationEnd => e instanceof NavigationEnd),
      map((e) => e.urlAfterRedirects),
      startWith(this.router.url),
    ),
    { initialValue: this.router.url },
  );

  ngOnInit(): void {
    firstValueFrom(this.catalogApi.listCatalog())
      .then((res) =>
        this.entries.set(
          res.entries
            .map((s) => s.entry)
            .filter((entry) => entry != null)
            .map((entry) => CatalogApiService.mapCatalogEntry(entry)),
        ),
      )
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));
  }

  readonly categories = CATEGORIES;

  readonly categoryIcon = categoryIcon;

  readonly totalProducts = computed(() => this.entries().length);

  readonly categoryCounts = computed<Record<string, number>>(() => {
    const counts: Record<string, number> = {};
    this.entries().forEach((entry) => {
      counts[entry.category] = (counts[entry.category] ?? 0) + 1;
    });
    return counts;
  });

  /** The address of a view, so every row is a real link. */
  readonly viewPath = (category: AssetCategory | 'all'): string =>
    category === 'all' ? `${CATALOG_PATH}/all` : `${CATALOG_PATH}/category/${viewSlug(category)}`;

  /**
   * Whether this row is the page you are on. Matched against the address, so a
   * product page (which is no view) lights up nothing.
   */
  isCurrent(category: AssetCategory | 'all'): boolean {
    const path = this.url().split('?')[0];
    if (category === 'all') return path === `${CATALOG_PATH}/all`;
    return path === this.viewPath(category);
  }

  /** A real link, routed in-app unless the click asks for a new tab or window. */
  goToView(event: Event, category: AssetCategory | 'all'): void {
    if (event instanceof MouseEvent) {
      if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey || event.button !== 0) {
        return;
      }
    }

    event.preventDefault();
    this.router.navigateByUrl(this.viewPath(category));
  }
}
