import {
  ChangeDetectionStrategy,
  Component,
  computed,
  CUSTOM_ELEMENTS_SCHEMA,
  effect,
  ElementRef,
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
import type { AssetStats } from '../../generated/v1/asset_pb';
import { RackSlotType } from '../../generated/v1/common_pb';
import InventoryApiService from './inventory-api.service';
import CatalogApiService from '../catalog/catalog-api.service';
import PlacementApiService, { RackOption } from './placement-api.service';
import {
  ASSET_STATUSES,
  ASSET_STATUSES_BY_ATTENTION,
  ASSET_STATUS_TAG_COLOR,
} from './asset-status';
import connectErrorMessage from '../../connect/error';
import parseValidationError from '../../connect/validation';
import SecondaryNavService from '../shell/secondary-nav.service';
import categoryIcon, { AssetCategory, CATEGORIES } from '../shared/asset-category';
import { viewSlug } from '../shared/section-views';
import { INVENTORY_PATH } from './inventory-views';
import InventoryNavComponent from './inventory-nav';
import openOnCreateRequest from '../shell/create-request';
import OverlayService from '../shell/overlay.service';
import InventoryStatsService from './inventory-stats.service';

export type { AssetCategory };

export type AssetStatus =
  'needs-repair' | 'decommissioned' | 'deployed' | 'available' | 'on-order' | 'requested';

/** Mirrors the proto AssetEventType enum (common.proto). */
export type AssetEventAction =
  | 'received'
  | 'deployed'
  | 'moved'
  | 'repair-sent'
  | 'repair-received'
  | 'decommissioned'
  | 'requested'
  | 'note';

export interface HistoryEntry {
  action: AssetEventAction;
  description: string;
  user: string;
  daysAgo: number;
}

export interface Asset {
  id: string;
  model: string;
  assetTag: string;
  category: AssetCategory;
  status: AssetStatus;
  notes: string;
  /** Hardware serial number. Empty for asset types that carry none. */
  serialNumber?: string;
  /** Warranty expiry as an ISO date (YYYY-MM-DD). Absent when not tracked. */
  warrantyExpiry?: string;
  /** Catalog entry the asset is an instance of. Absent for mock data. */
  deviceCatalogId?: string;
  /** Physical location. Tracked via Placement, so absent from the asset API. */
  datacenter?: string;
  rack?: string;
  parentId?: string;
}

export interface NoteComment {
  /** Note id when sourced from the API; absent for mock data. */
  id?: string;
  author: string;
  initials: string;
  daysAgo: number;
  content: string;
}

export interface CatalogEntry {
  id: string;
  model: string;
  manufacturer: string;
  partNumber?: string;
  category: AssetCategory;
  specs: Record<string, string>;
}

/** Catalog port-type enum keys, as used on the wire by the catalog API. */
export type PortTypeKey = 'network' | 'power_in' | 'power_out' | 'slot' | 'bay' | 'console';

/** Catalog port-direction enum keys. */
export type PortDirectionKey = 'in' | 'out' | 'bidir';

export interface PortDefinition {
  id: string;
  catalogEntryId: string;
  name: string;
  /** Port category enum key ({@link PortTypeKey}), or '' while a draft port is unsaved. */
  portType: string;
  /** Direction enum key ({@link PortDirectionKey}). */
  direction: string;
  /** Free-text connector/media (e.g. SFP+, QSFP28, IEC C13). */
  mediaType?: string;
  speedGbps?: number;
  powerWatts?: number;
  ordinal?: number;
}

export interface PortCompatibility {
  id: string;
  portDefinitionId: string;
  /** Category the port accepts. Always set. */
  compatibleCategory: AssetCategory;
  /** Specific catalog entry the compatibility is narrowed to. Empty for category-wide matches. */
  compatibleCatalogEntryId: string;
}

/**
 * What a row in the menu points at: everything, one status, or one category.
 * The first is not a status or a category with an "all" in it, which is why it
 * sits above both rather than at the head of each.
 */
type MenuKind = 'all' | 'status' | 'category';

@Component({
  selector: 'app-inventory',
  templateUrl: './inventory.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [InventoryNavComponent],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  // No styling of its own: the page inside paints the surface and owns the
  // layout, and styles.css takes this element out of the flow (display:
  // contents) so it cannot come between the pane and the page.
})
export default class InventoryComponent implements OnInit, AfterViewInit, OnDestroy {
  private readonly secondaryNav = inject(SecondaryNavService);

  /** This section's menu, handed to the shell for as long as the page is open. */
  private readonly secondaryNavTemplate = viewChild.required<TemplateRef<unknown>>('secondaryNav');

  ngAfterViewInit(): void {
    this.secondaryNav.set(this.secondaryNavTemplate());
  }

  ngOnDestroy(): void {
    this.secondaryNav.clear(this.secondaryNavTemplate());
  }

  private readonly inventoryApi = inject(InventoryApiService);

  private readonly catalogApi = inject(CatalogApiService);

  /** The asset form lives in the shell, so it can be opened from anywhere. */
  private readonly overlays = inject(OverlayService);

  /** Says when an asset changed somewhere else than this page. */
  private readonly assetChanges = inject(InventoryStatsService);

  private readonly placementApi = inject(PlacementApiService);

  readonly assets = signal<Asset[]>([]);

  /**
   * True while a new view's assets are on their way. Only a view change sets
   * it: the list you are looking at answers another question then, and showing
   * it until the new one lands means a long list flashing past on the way to a
   * short one. Typing in the search box does not, because there the list is
   * the same question narrowing.
   */
  readonly switchingView = signal(false);

  readonly catalog = signal<CatalogEntry[]>([]);

  private catalogById = new Map<string, CatalogEntry>();

  readonly stats = signal<AssetStats | null>(null);

  searchQuery = signal('');

  private readonly route = inject(ActivatedRoute);

  private readonly router = inject(Router);

  private readonly viewParams = toSignal(this.route.paramMap, {
    initialValue: convertToParamMap({}),
  });

  /**
   * Whether the address names a view. The section's own path (/inventory) means you
   * have opened the section and picked nothing yet, and then the pane beside
   * the menu says so rather than showing a list you did not ask for.
   */
  readonly hasSelection = computed(() => this.viewParams().get('view') !== null);

  private readonly queryParams = toSignal(this.route.queryParamMap, {
    initialValue: convertToParamMap({}),
  });

  /**
   * What the menu points at, read from the address. One choice, not two: the
   * menu is navigation, so picking a category takes you to the categories the
   * way a link takes you to a page, instead of narrowing what a status already
   * narrowed. Combining is what a filter does, and that belongs above the list.
   *
   * The address is where it lives rather than a signal, so a view can be linked
   * to, opened in a second tab and reached with the browser's back button.
   */
  readonly menuSelection = computed<{ kind: MenuKind; value: string }>(() => {
    const params = this.viewParams();
    const value = params.get('value') ?? '';
    switch (params.get('view')) {
      case 'status':
        return {
          kind: 'status',
          value: this.statuses.find((s) => viewSlug(s.value) === value)?.value ?? 'all',
        };
      case 'category':
        return {
          kind: 'category',
          value: this.categories.find((c) => viewSlug(c) === value) ?? 'all',
        };
      default:
        return { kind: 'all', value: 'all' };
    }
  });

  /**
   * The title of the page is the row you picked in the menu. The section name
   * is already in the menu's own heading and in the way back, so repeating it
   * above the list would say "Inventory" three times and never say which
   * assets you are looking at.
   */
  readonly viewTitle = computed(() => {
    const { kind, value } = this.menuSelection();
    if (kind === 'status')
      return this.statuses.find((s) => s.value === value)?.label ?? 'All assets';
    if (kind === 'category') return value;
    return 'All assets';
  });

  /** The address of a view, so every row in the menu is a real link. */
  readonly viewPath = (kind: MenuKind, value = ''): string =>
    kind === 'all' ? `${INVENTORY_PATH}/all` : `${INVENTORY_PATH}/${kind}/${viewSlug(value)}`;

  /**
   * Routes a click in-app while the row stays a real `<a href>`, so middle-click
   * and "open in new tab" keep working. Anything with a modifier is left to the
   * browser.
   */
  goToView(event: Event, kind: MenuKind, value = ''): void {
    if (event instanceof MouseEvent) {
      if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey || event.button !== 0) {
        return;
      }
    }

    event.preventDefault();
    this.router.navigateByUrl(this.viewPath(kind, value));
  }

  /** Back from the list is back to the menu, so the address says so too. */
  goToMenu(): void {
    this.router.navigateByUrl(INVENTORY_PATH);
  }

  /** The address of one asset, so a row is a real link. */
  readonly assetPath = (id: string): string => `${INVENTORY_PATH}/${id}`;

  /** Same trade as the menu rows: a real link, routed in-app without modifiers. */
  openAsset(event: Event, id: string): void {
    if (event instanceof MouseEvent) {
      if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey || event.button !== 0) {
        return;
      }
    }

    event.preventDefault();
    this.router.navigateByUrl(this.assetPath(id));
  }

  readonly statusFilter = computed<AssetStatus | 'all'>(() => {
    const selection = this.menuSelection();
    return selection.kind === 'status' ? (selection.value as AssetStatus) : 'all';
  });

  readonly categoryFilter = computed<AssetCategory | 'all'>(() => {
    const selection = this.menuSelection();
    return selection.kind === 'category' ? (selection.value as AssetCategory) : 'all';
  });

  /**
   * The status picked in the toolbar, as a query parameter rather than a signal.
   * The menu is one choice, so "which servers are broken" had nowhere to live:
   * you could pick the category or the status and never both. This is the
   * second axis, and it sits in the address so it can be linked to and so it
   * goes away by itself when you leave for another view.
   */
  readonly statusParam = computed<AssetStatus | 'all'>(() => {
    const value = this.queryParams().get('status') ?? '';
    return this.statuses.find((s) => viewSlug(s.value) === value)?.value ?? 'all';
  });

  /** What the list asks for: the status of the view, or the one in the toolbar. */
  readonly activeStatus = computed<AssetStatus | 'all'>(() => {
    const view = this.statusFilter();
    return view !== 'all' ? view : this.statusParam();
  });

  /** Nothing to narrow when the view already is one status. */
  readonly showStatusFilter = computed(() => this.statusFilter() === 'all');

  readonly statusFilterLabel = computed(() => {
    const status = this.statusParam();
    return status === 'all' ? 'Status' : this.statusLabel(status);
  });

  /** Puts the pick in the address, or takes it out again for "All". */
  setStatusParam(status: AssetStatus | 'all'): void {
    this.router.navigate([], {
      relativeTo: this.route,
      queryParams: { status: status === 'all' ? null : viewSlug(status) },
      queryParamsHandling: 'merge',
    });
  }

  /**
   * The order the rows are sorted in. See ASSET_STATUSES_BY_ATTENTION for why
   * that order.
   *
   * Alphabetical order was what the API offered and it means nothing here:
   * "Decommissioned" landing between "Available" and "Deployed" is a fact about
   * spelling, not about the rack.
   */
  private readonly attentionOrder: AssetStatus[] = ASSET_STATUSES_BY_ATTENTION.map(
    (status) => status.value,
  );

  /**
   * The rows in that order, then by model so identical machines sit together,
   * then by asset number so nothing moves between two loads of the same data.
   * Sorted here rather than server-side because the API has no field for it;
   * that holds as long as a view fetches its whole list at once, and needs a
   * sort field of its own once these lists start paginating.
   */
  readonly orderedAssets = computed(() => {
    const rank = (status: AssetStatus): number => this.attentionOrder.indexOf(status);
    return [...this.assets()].sort(
      (a, b) =>
        rank(a.status) - rank(b.status) ||
        a.model.localeCompare(b.model) ||
        a.assetTag.localeCompare(b.assetTag),
    );
  });

  /**
   * The menu in that same order, so the rows you pick from and the rows you get
   * are sorted by the same idea. The form's own status list keeps its lifecycle
   * order, which is what you want when you are setting a state rather than
   * scanning for the one that needs you.
   */
  readonly menuStatuses = computed(() =>
    [...this.statuses].sort(
      (a, b) => this.attentionOrder.indexOf(a.value) - this.attentionOrder.indexOf(b.value),
    ),
  );

  // ── CRUD state ─────────────────────────────────────────────────────────────
  editAsset = signal<Partial<Asset> | null>(null);

  /** Bound values for the edit form's <select>s (seeded on open, read on save). */
  readonly assetDeviceId = signal<string>('');

  readonly assetStatus = signal<AssetStatus>('available');

  readonly assetRackId = signal<string>('');

  /** The slot type as the enum the API takes; empty until one is picked. */
  readonly assetSlotType = signal<RackSlotType | ''>('');

  /** Rack placement of the asset being edited; null when adding or unplaced. */
  editPlacement = signal<{
    id: string;
    rackId: string;
    unit: number;
    slotType: RackSlotType;
  } | null>(null);

  /** The place you picked for the rack list, empty until you touch it. */
  readonly pickedLocation = signal<string>('');

  /** All racks, for the location picker. */
  readonly racks = signal<RackOption[]>([]);

  /** Racks grouped by datacenter, for the location <select> optgroups. */
  readonly racksByDatacenter = computed(() => {
    const groups = new Map<string, RackOption[]>();
    this.racks().forEach((rack) => {
      const list = groups.get(rack.datacenter) ?? [];
      list.push(rack);
      groups.set(rack.datacenter, list);
    });
    return [...groups.entries()]
      .map(([datacenter, racks]) => ({ datacenter, racks }))
      .sort((a, b) => a.datacenter.localeCompare(b.datacenter));
  });

  /** The places that have racks, in the order the groups come out. */
  readonly locations = computed(() => this.racksByDatacenter().map((group) => group.datacenter));

  /**
   * The place the rack list is limited to. Falls back to the first one, so the
   * rack picker always has something to show: a control you cannot use until
   * you have used another one first reads as broken.
   */
  readonly rackLocation = computed(() => this.pickedLocation() || this.locations()[0] || '');

  /** Only the racks that stand at the place you picked. */
  readonly racksAtLocation = computed(
    () =>
      this.racksByDatacenter().find((group) => group.datacenter === this.rackLocation())?.racks ??
      [],
  );

  readonly slotTypes: { value: RackSlotType; label: string }[] = [
    { value: RackSlotType.UNIT, label: 'Unit' },
    { value: RackSlotType.POWER, label: 'Power' },
    { value: RackSlotType.ZERO_U, label: 'Zero-U' },
  ];

  // ── Validation feedback ──────────────────────────────────────────────────────
  readonly invalidFields = signal<Record<string, string>>({});

  readonly formErrorMessage = signal<string | null>(null);

  private readonly assetSheetEl = viewChild<ElementRef>('assetSheet');

  private readonly fAssetTag = viewChild<ElementRef>('fAssetTag');

  private readonly fAssetSerial = viewChild<ElementRef>('fAssetSerial');

  private readonly fAssetWarranty = viewChild<ElementRef>('fAssetWarranty');

  private readonly fAssetRackUnit = viewChild<ElementRef>('fAssetRackUnit');

  private readonly fAssetNotes = viewChild<ElementRef>('fAssetNotes');

  constructor() {
    // An asset made from the add button in the bar is saved somewhere else, so
    // this list reads itself again when the shell says something changed.
    effect(() => {
      this.assetChanges.changed();
      untracked(() => {
        this.loadAssets();
        this.loadStats();
      });
    });
    // The add menu in the bar asks for this form through the address.
    openOnCreateRequest(() => this.openCreateAsset());
    // The view and the status filter both live in the address now, so the
    // address is what asks for a new query. This runs on arrival too, which is
    // where the first load comes from.
    effect(() => {
      this.menuSelection();
      this.statusParam();
      this.loadAssets(true);
    });

    toObservable(this.searchQuery)
      .pipe(
        skip(1),
        debounce((q) => timer(q ? 250 : 0)),
        distinctUntilChanged(),
        takeUntilDestroyed(),
      )
      .subscribe(() => this.reload());

    effect(() => {
      const el = this.assetSheetEl()?.nativeElement as { show?: () => void; hide?: () => void };
      if (this.editAsset() !== null) el?.show?.();
      else el?.hide?.();
    });
  }

  ngOnInit(): void {
    this.loadStats();
    this.loadRackOptions();

    firstValueFrom(this.catalogApi.listCatalog())
      .then((res) => {
        this.catalogById = new Map(
          res.entries
            .filter((s) => s.entry)
            .map((s) => {
              const entry = CatalogApiService.mapCatalogEntry(s.entry!);
              return [entry.id, entry] as const;
            }),
        );
        this.catalog.set([...this.catalogById.values()]);
      })
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)))
      .finally(() => this.loadAssets());
  }

  private loadRackOptions(): void {
    this.placementApi
      .listRackOptions()
      .then((racks) => this.racks.set(racks))
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));
  }

  readonly categories = CATEGORIES;

  readonly statuses = ASSET_STATUSES;

  private loadAssets(viewChange = false): void {
    if (viewChange) this.switchingView.set(true);
    firstValueFrom(
      this.inventoryApi.listAssets({
        search: this.searchQuery().trim(),
        status: this.activeStatus(),
        category: this.categoryFilter(),
        // The API needs a direction; which one does not matter, because the
        // list is put in its own order here (see orderedAssets).
        sortDirection: 'asc',
      }),
    )
      .then((res) =>
        this.assets.set(res.assets.map((a) => InventoryApiService.mapAsset(a, this.catalogById))),
      )
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)))
      .finally(() => this.switchingView.set(false));
  }

  private loadStats(): void {
    firstValueFrom(this.inventoryApi.getAssetStats())
      .then((res) => this.stats.set(res.stats ?? null))
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));
  }

  /** Re-query the list after a filter or sort change. */
  private reload(): void {
    this.loadAssets();
  }

  // ── Summary counts (from org-wide stats) ───────────────────────────────────

  readonly statusCounts = computed<Partial<Record<AssetStatus | 'all', number>>>(() => {
    const s = this.stats();
    if (!s) return {};
    return {
      all: s.total,
      deployed: s.deployed,
      available: s.available,
      'on-order': s.onOrder,
      requested: s.requested,
      'needs-repair': s.needsRepair,
      decommissioned: s.decommissioned,
    };
  });

  /**
   * What an empty view says. Not "0 of 70": a view is not a filter over
   * everything, it is a list of its own, and a search that finds nothing is a
   * different sentence from a category nobody has put anything in yet.
   */
  readonly emptyText = computed(() => {
    const query = this.searchQuery().trim();
    if (query) return `No results for "${query}"`;
    const { kind, value } = this.menuSelection();
    // A filter is not the view: "No Switch assets" would say the category is
    // empty while it is the filter on top of it that found nothing.
    const status = this.statusParam();
    if (status !== 'all') {
      const label = this.statusLabel(status).toLowerCase();
      return kind === 'category' ? `No ${label} ${value} assets` : `No ${label} assets`;
    }
    if (kind === 'category') return `No ${value} assets`;
    if (kind === 'status') return `No ${this.statusLabel(value as AssetStatus)} assets`;
    return 'No assets';
  });

  /**
   * How many rows this view holds. Not "7 of 70": the view is a list of its own,
   * not a slice of everything, and the only place a denominator means something
   * is a search, which narrows the view you are in.
   */
  readonly listSummary = computed(() => {
    const shown = this.assets().length;
    const noun = shown === 1 ? 'asset' : 'assets';
    return this.searchQuery().trim()
      ? `${shown} ${shown === 1 ? 'result' : 'results'}`
      : `${shown} ${noun}`;
  });

  readonly totalCount = computed(() => this.stats()?.total ?? 0);

  readonly deployedCount = computed(() => this.stats()?.deployed ?? 0);

  readonly availableCount = computed(() => this.stats()?.available ?? 0);

  readonly issuesCount = computed(() => {
    const s = this.stats();
    return s ? s.needsRepair + s.decommissioned : 0;
  });

  // ── Filter / sort actions ──────────────────────────────────────────────────

  /** Whether this menu row is the one the list is showing. */
  isMenuSelection(kind: MenuKind, value = 'all'): boolean {
    return this.menuSelection().kind === kind && this.menuSelection().value === value;
  }

  // ── CRUD actions ───────────────────────────────────────────────────────────

  isFieldInvalid(field: string): boolean {
    return field in this.invalidFields();
  }

  fieldError(field: string): string {
    return this.invalidFields()[field] ?? '';
  }

  private clearErrors(): void {
    this.invalidFields.set({});
    this.formErrorMessage.set(null);
  }

  private handleError(err: unknown): void {
    const { fields, message } = parseValidationError(err);
    this.invalidFields.set(fields);
    this.formErrorMessage.set(message);
  }

  /** Both routes into the form open the one the shell holds: it outlives this
   *  page, and the add button in the bar opens the same one. */
  openCreateAsset(): void {
    const status = this.activeStatus();
    this.overlays.newAsset(status === 'all' ? undefined : status);
  }

  openEditAsset(asset: Asset): void {
    this.overlays.editAsset(asset);
  }

  statusLabel(status: AssetStatus): string {
    return this.statuses.find((s) => s.value === status)?.label ?? status;
  }

  readonly statusTagColor = (status: AssetStatus): string => ASSET_STATUS_TAG_COLOR[status];

  // A bare indicator dot in the status filter menu, not a tag.
  readonly statusDotClass = (status: AssetStatus): string => {
    const map: Record<AssetStatus, string> = {
      deployed: 'bg-teal-400',
      available: 'bg-green-400',
      'needs-repair': 'bg-amber-400',
      decommissioned: 'bg-slate-400',
      'on-order': 'bg-blue-400',
      requested: 'bg-purple-400',
    };
    return map[status];
  };

  readonly categoryIcon = categoryIcon;
}
