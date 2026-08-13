import {
  ChangeDetectionStrategy,
  Component,
  computed,
  CUSTOM_ELEMENTS_SCHEMA,
  effect,
  inject,
  OnInit,
  signal,
  untracked,
  viewChild,
  TemplateRef,
  AfterViewInit,
  OnDestroy,
} from '@angular/core';
import { takeUntilDestroyed, toObservable, toSignal } from '@angular/core/rxjs-interop';
import { ActivatedRoute, convertToParamMap, Router } from '@angular/router';
import { debounce, distinctUntilChanged, firstValueFrom, skip, timer } from 'rxjs';
import { AssetCategory, CatalogEntry } from '../inventory/inventory';
import CatalogApiService from './catalog-api.service';
import InventoryApiService from '../inventory/inventory-api.service';
import connectErrorMessage from '../../connect/error';
import { AssetStatus as ProtoStatus } from '../../generated/v1/common_pb';
import type { Asset as ProtoAsset } from '../../generated/v1/asset_pb';
import SecondaryNavService from '../shell/secondary-nav.service';
import categoryIcon, { CATEGORIES } from '../shared/asset-category';
import { viewSlug } from '../shared/section-views';
import { CATALOG_PATH } from './catalog-views';
import CatalogNavComponent from './catalog-nav';
import OverlayService from '../shell/overlay.service';

interface CatalogRow {
  entry: CatalogEntry;
  total: number;
  deployed: number;
  available: number;
  issues: number;
}

@Component({
  selector: 'app-catalog',
  templateUrl: './catalog.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [CatalogNavComponent],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  // No styling of its own: the page inside paints the surface and owns the
  // layout, and styles.css takes this element out of the flow (display:
  // contents) so it cannot come between the pane and the page.
})
export default class CatalogComponent implements OnInit, AfterViewInit, OnDestroy {
  private readonly secondaryNav = inject(SecondaryNavService);

  /** This section's menu, handed to the shell for as long as the page is open. */
  private readonly secondaryNavTemplate = viewChild.required<TemplateRef<unknown>>('secondaryNav');

  ngAfterViewInit(): void {
    this.secondaryNav.set(this.secondaryNavTemplate());
  }

  ngOnDestroy(): void {
    this.secondaryNav.clear(this.secondaryNavTemplate());
  }

  private readonly catalogApi = inject(CatalogApiService);

  /** The product form is the shell's, so this page only asks it to open. */
  readonly overlays = inject(OverlayService);

  private readonly inventoryApi = inject(InventoryApiService);

  /** All assets, used to derive instance counts per catalog entry. */
  private readonly assets = signal<ProtoAsset[]>([]);

  searchQuery = signal('');

  private readonly route = inject(ActivatedRoute);

  private readonly router = inject(Router);

  private readonly viewParams = toSignal(this.route.paramMap, {
    initialValue: convertToParamMap({}),
  });

  /**
   * Whether the address names a view. The section's own path (/catalog) means you
   * have opened the section and picked nothing yet, and then the pane beside
   * the menu says so rather than showing a list you did not ask for.
   */
  readonly hasSelection = computed(() => this.viewParams().get('view') !== null);

  /**
   * Which category the list is showing, read from the address. The menu is
   * navigation, so a category can be linked to, opened in a second tab and
   * reached with the browser's back button.
   */
  readonly categoryFilter = computed<AssetCategory | 'all'>(() => {
    const params = this.viewParams();
    if (params.get('view') !== 'category') return 'all';
    const value = params.get('value') ?? '';
    return this.categories.find((c) => viewSlug(c) === value) ?? 'all';
  });

  /**
   * The title of the page is the row you picked in the menu. The section name
   * is already in the menu's own heading and in the way back, so repeating it
   * above the list would say "Catalog" three times and never say which products
   * you are looking at.
   */
  readonly viewTitle = computed(() => {
    const cat = this.categoryFilter();
    return cat === 'all' ? 'All products' : cat;
  });

  /** The address of a view, so every row in the menu is a real link. */
  readonly viewPath = (category: AssetCategory | 'all'): string =>
    category === 'all' ? `${CATALOG_PATH}/all` : `${CATALOG_PATH}/category/${viewSlug(category)}`;

  readonly categories = CATEGORIES;

  // ── Mutable catalog list ───────────────────────────────────────────────────
  readonly mutableCatalog = signal<CatalogEntry[]>([]);

  constructor() {
    toObservable(this.searchQuery)
      .pipe(
        skip(1),
        debounce((q) => timer(q ? 250 : 0)),
        distinctUntilChanged(),
        takeUntilDestroyed(),
      )
      .subscribe((search) => this.loadCatalog(search));

    // The product form lives in the shell, so this page only has to notice that
    // something was written and read the list again.
    effect(() => {
      this.catalogApi.revision();
      untracked(() => this.loadCatalog(this.searchQuery().trim() || undefined));
    });
  }

  ngOnInit(): void {
    this.loadAssets();
  }

  private loadAssets(): void {
    firstValueFrom(
      this.inventoryApi.listAssets({ status: 'all', category: 'all', sortDirection: 'asc' }),
    )
      .then((res) => this.assets.set(res.assets))
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));
  }

  private loadCatalog(search?: string): void {
    firstValueFrom(this.catalogApi.listCatalog(search))
      .then((res) =>
        this.mutableCatalog.set(
          res.entries.map((s) => CatalogApiService.mapCatalogEntry(s.entry!)),
        ),
      )
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));
  }

  private readonly allRows = computed<CatalogRow[]>(() => {
    const counts = new Map<string, Omit<CatalogRow, 'entry'>>();
    this.assets().forEach((a) => {
      const c = counts.get(a.deviceCatalogId) ?? { total: 0, deployed: 0, available: 0, issues: 0 };
      c.total += 1;
      if (a.status === ProtoStatus.DEPLOYED) c.deployed += 1;
      else if (a.status === ProtoStatus.AVAILABLE) c.available += 1;
      if (a.status === ProtoStatus.NEEDS_REPAIR || a.status === ProtoStatus.DECOMMISSIONED) {
        c.issues += 1;
      }
      counts.set(a.deviceCatalogId, c);
    });
    return this.mutableCatalog().map((entry) => ({
      entry,
      ...(counts.get(entry.id) ?? { total: 0, deployed: 0, available: 0, issues: 0 }),
    }));
  });

  readonly rows = computed<CatalogRow[]>(() => {
    const cat = this.categoryFilter();
    if (cat === 'all') return this.allRows();
    return this.allRows().filter((row) => row.entry.category === cat);
  });

  /**
   * What an empty view says. Not "0 of 10": a category is not a filter over
   * everything, it is a list of its own, and a search that finds nothing is a
   * different sentence from a category nobody has put anything in yet.
   */
  readonly emptyText = computed(() => {
    const query = this.searchQuery().trim();
    if (query) return `No results for "${query}"`;
    const category = this.categoryFilter();
    return category === 'all' ? 'No products' : `No ${category} products`;
  });

  /**
   * How many rows this view holds. Not "2 of 10": a category is a list of its
   * own, not a slice of everything, and the only place a denominator means
   * something is a search, which narrows the view you are in.
   */
  readonly listSummary = computed(() => {
    const shown = this.rows().length;
    const noun = shown === 1 ? 'product' : 'products';
    return this.searchQuery().trim()
      ? `${shown} ${shown === 1 ? 'result' : 'results'}`
      : `${shown} ${noun}`;
  });

  readonly categoryCounts = computed(() => {
    const counts: Record<string, number> = {};
    this.allRows().forEach((row) => {
      counts[row.entry.category] = (counts[row.entry.category] ?? 0) + 1;
    });
    return counts;
  });

  /**
   * Routes a click in-app while the row stays a real `<a href>`, so middle-click
   * and "open in new tab" keep working. Anything with a modifier is left to the
   * browser.
   */
  selectCategory(event: Event, category: AssetCategory | 'all'): void {
    if (event instanceof MouseEvent) {
      if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey || event.button !== 0) {
        return;
      }
    }

    event.preventDefault();
    this.router.navigateByUrl(this.viewPath(category));
  }

  /** Back from the list is back to the menu, so the address says so too. */
  goToMenu(): void {
    this.router.navigateByUrl(CATALOG_PATH);
  }

  /** The address of one product, so a row is a real link. */
  readonly entryPath = (id: string): string => `${CATALOG_PATH}/${id}`;

  /** Same trade as the menu rows: a real link, routed in-app without modifiers. */
  openEntry(event: Event, id: string): void {
    if (event instanceof MouseEvent) {
      if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey || event.button !== 0) {
        return;
      }
    }

    event.preventDefault();
    this.router.navigateByUrl(this.entryPath(id));
  }

  readonly categoryIcon = categoryIcon;
}
