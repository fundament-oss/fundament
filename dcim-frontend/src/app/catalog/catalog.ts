import {
  ChangeDetectionStrategy,
  Component,
  computed,
  CUSTOM_ELEMENTS_SCHEMA,
  effect,
  inject,
  OnInit,
  signal,
  viewChild,
  TemplateRef,
  AfterViewInit,
  OnDestroy,
} from '@angular/core';
import { takeUntilDestroyed, toObservable, toSignal } from '@angular/core/rxjs-interop';
import { FormsModule } from '@angular/forms';
import { ActivatedRoute, convertToParamMap, Router } from '@angular/router';
import { debounce, distinctUntilChanged, firstValueFrom, skip, timer } from 'rxjs';
import { AssetCategory, CatalogEntry } from '../inventory/inventory';
import CatalogApiService from './catalog-api.service';
import InventoryApiService from '../inventory/inventory-api.service';
import connectErrorMessage from '../../connect/error';
import parseValidationError from '../../connect/validation';
import { AssetStatus as ProtoStatus } from '../../generated/v1/common_pb';
import type { Asset as ProtoAsset } from '../../generated/v1/asset_pb';
import DropdownSyncDirective from '../shared/dropdown-sync.directive';
import SecondaryNavService from '../shell/secondary-nav.service';
import categoryIcon, { CATEGORIES } from '../shared/asset-category';
import { viewSlug } from '../shared/section-views';
import { CATALOG_PATH } from './catalog-views';
import CatalogNavComponent from './catalog-nav';

interface NativeElementRef {
  nativeElement: { value: string; show?: () => void; hide?: () => void };
}

interface CatalogRow {
  entry: CatalogEntry;
  total: number;
  deployed: number;
  available: number;
  issues: number;
}

type InvalidFields = Record<string, string>;

@Component({
  selector: 'app-catalog',
  templateUrl: './catalog.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [FormsModule, DropdownSyncDirective, CatalogNavComponent],
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

  // ── CRUD state ─────────────────────────────────────────────────────────────
  editEntry = signal<Partial<CatalogEntry> | null>(null);

  entryCategory = signal<AssetCategory>('Server');

  entryErrorMessage = signal<string | null>(null);

  invalidFields = signal<InvalidFields>({});

  specRows = signal<{ key: string; value: string }[]>([]);

  private readonly entrySheetEl = viewChild<NativeElementRef>('entrySheet');

  private readonly fEntryModel = viewChild<NativeElementRef>('fEntryModel');

  private readonly fEntryMfr = viewChild<NativeElementRef>('fEntryMfr');

  private readonly fEntryPart = viewChild<NativeElementRef>('fEntryPart');

  constructor() {
    toObservable(this.searchQuery)
      .pipe(
        skip(1),
        debounce((q) => timer(q ? 250 : 0)),
        distinctUntilChanged(),
        takeUntilDestroyed(),
      )
      .subscribe((search) => this.loadCatalog(search));

    effect(() => {
      const el = this.entrySheetEl()?.nativeElement;
      if (this.editEntry() !== null) el?.show?.();
      else el?.hide?.();
    });
  }

  ngOnInit(): void {
    this.loadCatalog();
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

  // ── CRUD actions ───────────────────────────────────────────────────────────

  openCreateEntry(): void {
    this.clearEntryErrors();
    this.editEntry.set({
      id: '',
      model: '',
      manufacturer: '',
      partNumber: '',
      category: 'Server',
      specs: {},
    });
    this.entryCategory.set('Server');
    this.specRows.set([{ key: '', value: '' }]);
  }

  closeEntryForm(): void {
    this.editEntry.set(null);
  }

  addSpecRow(): void {
    this.specRows.update((rows) => [...rows, { key: '', value: '' }]);
  }

  removeSpecRow(index: number): void {
    this.specRows.update((rows) => rows.filter((_, i) => i !== index));
  }

  updateSpecKey(index: number, event: Event): void {
    const val = (event.target as HTMLInputElement).value;
    this.specRows.update((rows) => rows.map((r, i) => (i === index ? { ...r, key: val } : r)));
  }

  updateSpecVal(index: number, event: Event): void {
    const val = (event.target as HTMLInputElement).value;
    this.specRows.update((rows) => rows.map((r, i) => (i === index ? { ...r, value: val } : r)));
  }

  saveEntry(): void {
    const form = this.editEntry();
    if (!form) return;

    this.clearEntryErrors();

    const model = this.fEntryModel()?.nativeElement.value ?? '';
    const manufacturer = this.fEntryMfr()?.nativeElement.value ?? '';
    const partNumber = this.fEntryPart()?.nativeElement.value ?? form.partNumber ?? '';
    const category = this.entryCategory();
    const specs: Record<string, string> = {};

    this.specRows().forEach((row) => {
      if (row.key.trim()) specs[row.key.trim()] = row.value;
    });

    const entry: CatalogEntry = {
      id: form.id || '',
      model,
      manufacturer,
      partNumber,
      category,
      specs,
    };

    if (form.id) {
      firstValueFrom(this.catalogApi.updateCatalogEntry(entry))
        .then(() => {
          this.mutableCatalog.update((list) => list.map((e) => (e.id === form.id ? entry : e)));
          this.editEntry.set(null);
        })
        .catch((err) => this.handleEntryError(err));
    } else {
      firstValueFrom(this.catalogApi.createCatalogEntry(entry))
        .then((res) => {
          this.mutableCatalog.update((list) => [...list, { ...entry, id: res.catalogEntryId }]);
          this.editEntry.set(null);
        })
        .catch((err) => this.handleEntryError(err));
    }
  }

  /** Returns true when the given proto field name has a validation error. */
  isFieldInvalid(field: string): boolean {
    return field in this.invalidFields();
  }

  /** Returns the validation message for a proto field, or '' when valid. */
  fieldError(field: string): string {
    return this.invalidFields()[field] ?? '';
  }

  private clearEntryErrors(): void {
    this.invalidFields.set({});
    this.entryErrorMessage.set(null);
  }

  private handleEntryError(err: unknown): void {
    const { fields, message } = parseValidationError(err);
    this.invalidFields.set(fields);
    this.entryErrorMessage.set(message);
  }

  readonly categoryIcon = categoryIcon;
}
