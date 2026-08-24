import {
  ChangeDetectionStrategy,
  Component,
  computed,
  effect,
  inject,
  OnInit,
  signal,
  untracked,
  viewChild,
  CUSTOM_ELEMENTS_SCHEMA,
  TemplateRef,
  AfterViewInit,
  OnDestroy,
} from '@angular/core';
import { Title } from '@angular/platform-browser';
import { ActivatedRoute, ParamMap, Router, RouterLink } from '@angular/router';
import { firstValueFrom, map } from 'rxjs';
import { toSignal } from '@angular/core/rxjs-interop';
import { pageTitle } from '../shell/page-title';
import DatacenterApiService from './datacenter-api.service';
import DatacenterListService from './datacenter-list.service';
import DatacenterNavComponent from './datacenter-nav';
import DatacenterDetailComponent from './datacenter-detail/datacenter-detail';
import PlacementApiService from '../inventory/placement-api.service';
import CatalogApiService from '../catalog/catalog-api.service';
import { ASSET_CLIENT } from '../../connect/tokens';
import connectErrorMessage from '../../connect/error';
import { parseRackHeight } from '../racks/catalog-helpers';
import IsometricCanvasComponent from './isometric-canvas';
import { DatacenterInfo, DatacenterStatus, RackCell, statusTagColor } from './datacenter.model';
import SecondaryNavService from '../shell/secondary-nav.service';
import { viewSlug } from '../shared/section-views';
import openOnCreateRequest from '../shell/create-request';
import OverlayService from '../shell/overlay.service';

interface NativeElementRef {
  nativeElement: { value: string; show?: () => void; hide?: () => void };
}

interface DcStats {
  rackCount: number;
  deviceCount: number;
  totalPowerKw: number;
  capacityPct: number;
}

// ── Component ─────────────────────────────────────────────────────────────────

@Component({
  selector: 'app-datacenters',
  templateUrl: './datacenters.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [
    RouterLink,
    IsometricCanvasComponent,
    DatacenterNavComponent,
    DatacenterDetailComponent,
  ],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
})
export default class DatacentersComponent implements OnInit, AfterViewInit, OnDestroy {
  private readonly secondaryNav = inject(SecondaryNavService);

  /** This section's menu, handed to the shell for as long as the page is open. */
  private readonly secondaryNavTemplate = viewChild.required<TemplateRef<unknown>>('secondaryNav');

  ngAfterViewInit(): void {
    this.secondaryNav.set(this.secondaryNavTemplate());
  }

  ngOnDestroy(): void {
    this.secondaryNav.clear(this.secondaryNavTemplate());
  }

  private readonly router = inject(Router);

  private readonly title = inject(Title);

  private readonly route = inject(ActivatedRoute);

  private readonly dcApi = inject(DatacenterApiService);

  private readonly placementApi = inject(PlacementApiService);

  private readonly catalogApi = inject(CatalogApiService);

  private readonly assetClient = inject(ASSET_CLIENT);

  private readonly list = inject(DatacenterListService);

  /** The forms that make or change a data center live in the shell. */
  private readonly overlays = inject(OverlayService);

  /** The list this section shares, so a step to another page does not lose it.
   *  Writable: creating, renaming and deleting one all write straight into it. */
  readonly mutableDcs = this.list.datacenters;

  readonly dcsLoaded = this.list.loaded;

  /** Which data center the address names, by its short name: /datacenters/ams1.
   *  A data center is a place, so it deserves a link of its own. */
  readonly slug = toSignal(
    this.route.paramMap.pipe(map((params: ParamMap) => params.get('slug') ?? '')),
    { initialValue: '' },
  );

  /**
   * Whether the layout editor is open, which the address says: see the matcher.
   * A sheet with an address rather than a page, so it can be linked to and the
   * back button closes it, without the editor becoming somewhere you navigate
   * to and have to find your way back from.
   */
  private readonly layoutSheetEl = viewChild<NativeElementRef>('layoutSheet');

  readonly layoutOpen = toSignal(
    this.route.paramMap.pipe(map((params: ParamMap) => params.get('overlay') === 'layout')),
    { initialValue: false },
  );

  /** The floor one level down: the rooms and the rack rows inside them, where
   *  they are made and renamed. */
  openLayout(dc: DatacenterInfo): void {
    this.router.navigate(['/data-centers', viewSlug(dc.name), 'layout']);
  }

  /** Back to the data center underneath, which is the address without the
   *  editor on top. */
  closeLayout(): void {
    const dc = this.currentDc();
    if (dc) this.router.navigate(['/data-centers', viewSlug(dc.name)]);
  }

  /** Nothing is picked without a slug: /datacenters is the list, not the first
   *  data center in it. Opening the section should not open a place as well. */
  readonly selectedDcId = computed(
    () => this.mutableDcs().find((dc) => viewSlug(dc.name) === this.slug())?.id ?? '',
  );

  viewMode = signal<'map' | 'isometric'>('map');

  hoveredRackId = signal<string | null>(null);

  tooltipX = signal(0);

  tooltipY = signal(0);

  // ── Floor layout (loaded per datacenter from the API) ────────────────────────
  readonly rackCells = signal<RackCell[]>([]);

  readonly dcStats = signal<DcStats>({
    rackCount: 0,
    deviceCount: 0,
    totalPowerKw: 0,
    capacityPct: 0,
  });

  // ── CRUD state ─────────────────────────────────────────────────────────────
  deleteTarget = signal<DatacenterInfo | null>(null);

  private readonly deleteModalEl = viewChild<NativeElementRef>('deleteModal');

  constructor() {
    // The address decides whether the editor is open, so the sheet follows it
    // rather than the other way round: a link that carries it opens it, and the
    // back button closes it without the page having to know it was pressed.
    effect(() => {
      const el = this.layoutSheetEl()?.nativeElement;
      if (this.layoutOpen()) el?.show?.();
      else el?.hide?.();
    });
    // The add button in the bar opens this form itself, so the page only has
    // to honour a link that asks for it.
    openOnCreateRequest(() => this.openCreateDc());
    // The tab says which one you have open, not just which section.
    effect(() => {
      const name = this.currentDc()?.name;
      if (name) this.title.setTitle(pageTitle(name));
    });
    effect(() => {
      const el = this.deleteModalEl()?.nativeElement;
      if (this.deleteTarget() !== null) el?.show?.();
      else el?.hide?.();
    });
    // The address says which data center is on screen, so the floor is read
    // again whenever it moves — including the first time, once the list of data
    // centers has landed.
    effect(() => {
      const id = this.selectedDcId();
      untracked(() => {
        this.hoveredRackId.set(null);
        if (id) this.loadFloor(id).catch(() => undefined);
      });
    });
  }

  ngOnInit(): void {
    this.list.load();
  }

  readonly currentDc = computed(() =>
    this.mutableDcs().find((dc) => dc.id === this.selectedDcId()),
  );

  /**
   * Loads the floor layout for a site: rack rows (grid rows), racks (grid
   * columns via position_in_row), and per-rack stats derived from placements +
   * catalog (rack units → fill %, power draw → power).
   */
  private async loadFloor(siteId: string): Promise<void> {
    if (!siteId) {
      this.rackCells.set([]);
      this.dcStats.set({ rackCount: 0, deviceCount: 0, totalPowerKw: 0, capacityPct: 0 });
      return;
    }
    try {
      const [rowsRes, racksRes, catalogRes, assetsRes] = await Promise.all([
        firstValueFrom(this.dcApi.listRackRowsBySite(siteId)),
        firstValueFrom(this.dcApi.listRacksBySite(siteId)),
        firstValueFrom(this.catalogApi.listCatalog()),
        firstValueFrom(this.assetClient.listAssets({})),
      ]);

      const rowNameById = new Map(rowsRes.rackRows.map((r) => [r.id, r.name]));
      const racks = racksRes.racks
        .map((summary) => summary.rack)
        .filter((r) => r != null)
        .map((r) => DatacenterApiService.mapRack(r));

      // Catalog id → rack units occupied + nominal power draw.
      const catalogStats = new Map<string, { units: number; powerW: number }>();
      catalogRes.entries.forEach((e) => {
        if (!e.entry) return;
        const units = e.entry.rackUnits || parseRackHeight(e.entry.specs);
        catalogStats.set(e.entry.id, { units, powerW: e.entry.powerDrawW });
      });
      const catalogByAsset = new Map(assetsRes.assets.map((a) => [a.id, a.deviceCatalogId]));

      const placementArrays = await Promise.all(
        racks.map((r) => firstValueFrom(this.placementApi.listPlacementsByRack(r.id))),
      );

      let totalUsedU = 0;
      let totalCapacityU = 0;
      let totalPowerW = 0;
      let deviceCount = 0;

      const cells: RackCell[] = racks.map((rack, i) => {
        const placements = placementArrays[i].placements.filter((p) => p.location.case === 'rack');
        const used = placements.reduce(
          (acc, p) => {
            const catId = catalogByAsset.get(p.assetId);
            const stats = catId ? catalogStats.get(catId) : undefined;
            return {
              units: acc.units + (stats?.units ?? 0),
              powerW: acc.powerW + (stats?.powerW ?? 0),
            };
          },
          { units: 0, powerW: 0 },
        );
        totalUsedU += used.units;
        totalCapacityU += rack.totalU;
        totalPowerW += used.powerW;
        deviceCount += placements.length;
        return {
          rackId: rack.id,
          rackName: rack.name,
          row: rowNameById.get(rack.rowId) ?? '?',
          col: rack.positionInRow,
          fillPct: rack.totalU > 0 ? Math.round((used.units / rack.totalU) * 100) : 0,
          deviceCount: placements.length,
          powerW: used.powerW,
        };
      });

      this.rackCells.set(cells);
      this.dcStats.set({
        rackCount: racks.length,
        deviceCount,
        totalPowerKw: totalPowerW / 1000,
        capacityPct: totalCapacityU > 0 ? Math.round((totalUsedU / totalCapacityU) * 100) : 0,
      });
    } catch (err) {
      // eslint-disable-next-line no-console
      console.error(connectErrorMessage(err));
    }
  }

  // All rack rows grouped for the map/isometric views.
  readonly floorRows = computed((): Map<string, RackCell[]> => {
    const floorMap = new Map<string, RackCell[]>();
    this.rackCells().forEach((cell) => {
      const row = floorMap.get(cell.row) ?? [];
      row.push(cell);
      floorMap.set(cell.row, row);
    });
    floorMap.forEach((cells, key) => {
      floorMap.set(
        key,
        [...cells].sort((a, b) => a.col - b.col),
      );
    });
    return floorMap;
  });

  readonly rowKeys = computed(() => [...this.floorRows().keys()].sort());

  readonly hoveredCell = computed(() => {
    const id = this.hoveredRackId();
    return id ? (this.rackCells().find((c) => c.rackId === id) ?? null) : null;
  });

  readonly firstRackRoute = computed(() => {
    const id = this.rackCells()[0]?.rackId;
    return id ? ['/racks', id] : ['/racks'];
  });

  // ── Color helpers ──────────────────────────────────────────────────────────

  readonly rackCellClass = (): string =>
    'bg-emerald-50 dark:bg-emerald-950 border-emerald-300 dark:border-emerald-700 text-emerald-700 dark:text-emerald-300 hover:border-emerald-500 cursor-pointer';

  readonly rackFillBarClass = (): string => 'bg-emerald-200 dark:bg-emerald-900';

  readonly statusTagColor = statusTagColor;

  readonly statusLabel = (status: DatacenterStatus): string => {
    switch (status) {
      case 'operational':
        return 'Operational';
      case 'degraded':
        return 'Degraded';
      case 'maintenance':
        return 'Maintenance';
      default:
        return '';
    }
  };

  // ── CRUD actions ───────────────────────────────────────────────────────────

  /** Both routes into the form open the one the shell holds: it has to survive
   *  the page, and the add button in the bar opens the same one. */
  openCreateDc(): void {
    this.overlays.newDatacenter();
  }

  openEditDc(dc: DatacenterInfo): void {
    this.overlays.editDatacenter(dc);
  }

  openDeleteDc(dc: DatacenterInfo): void {
    this.deleteTarget.set(dc);
  }

  cancelDelete(): void {
    this.deleteTarget.set(null);
  }

  confirmDeleteDc(): void {
    const target = this.deleteTarget();
    if (!target) return;
    firstValueFrom(this.dcApi.deleteSite(target.id))
      .then(() => {
        this.mutableDcs.update((list) => list.filter((dc) => dc.id !== target.id));
        if (this.selectedDcId() === target.id) {
          const remaining = this.mutableDcs();
          if (remaining[0]) this.selectDc(remaining[0].id);
        }
        this.deleteTarget.set(null);
      })
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));
  }

  // ── Actions ────────────────────────────────────────────────────────────────

  /** Picking one puts it in the address; the floor follows from there. */
  selectDc(id: string): void {
    const dc = this.mutableDcs().find((d) => d.id === id);
    this.router.navigate(['/data-centers', dc ? viewSlug(dc.name) : '']);
  }

  onMapMouseMove(event: MouseEvent): void {
    this.tooltipX.set(event.clientX + 20);
    this.tooltipY.set(event.clientY + 20);
  }

  navigateToRack(rackId: string): void {
    this.router.navigate(['/racks', rackId]);
  }



  /** Opens the rack section on the first rack of this data center. */
  openRackView(): void {
    this.router.navigate(this.firstRackRoute());
  }

  /** The tasks of this data center: its name is a tag on every task that
   *  happens here, so the tag view is the list. */
  openTaskManagement(dc: DatacenterInfo): void {
    this.router.navigate(['/tasks', 'tag', dc.name]);
  }

  /** Street, city and country on one line: an address is the whole thing, and
   *  the city already stands in the menu row beside it. */
  readonly fullAddress = (dc: DatacenterInfo): string =>
    [dc.address, dc.city, dc.country].filter((part) => !!part).join(', ');

  readonly formatPowerKw = (kw: number): string => `${kw.toFixed(1)} kW`;
}
