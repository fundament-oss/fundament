import {
  ChangeDetectionStrategy,
  Component,
  computed,
  effect,
  inject,
  OnInit,
  signal,
  viewChild,
  CUSTOM_ELEMENTS_SCHEMA,
} from '@angular/core';
import { FormsModule } from '@angular/forms';
import { Router, RouterLink } from '@angular/router';
import { firstValueFrom } from 'rxjs';
import DcSelectorComponent from '../shared/dc-selector';
import DatacenterApiService from './datacenter-api.service';
import PlacementApiService from '../inventory/placement-api.service';
import CatalogApiService from '../catalog/catalog-api.service';
import { ASSET_CLIENT } from '../../connect/tokens';
import connectErrorMessage from '../../connect/error';
import parseValidationError from '../../connect/validation';
import { parseRackHeight } from '../racks/catalog-helpers';
import IsometricCanvasComponent from './isometric-canvas';
import { DatacenterInfo, DatacenterStatus, RackCell } from './datacenter.model';
import DropdownSyncDirective from '../shared/dropdown-sync.directive';

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
    FormsModule,
    DcSelectorComponent,
    IsometricCanvasComponent,
    DropdownSyncDirective,
  ],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  host: { class: 'flex flex-col bg-white text-slate-900' },
})
export default class DatacentersComponent implements OnInit {
  private readonly router = inject(Router);

  private readonly dcApi = inject(DatacenterApiService);

  private readonly placementApi = inject(PlacementApiService);

  private readonly catalogApi = inject(CatalogApiService);

  private readonly assetClient = inject(ASSET_CLIENT);

  // ── DC list (loaded from the API) ──────────────────────────────────────────
  readonly mutableDcs = signal<DatacenterInfo[]>([]);

  selectedDcId = signal('');

  viewMode = signal<'map' | 'isometric'>('map');

  hoveredRackId = signal<string | null>(null);

  tooltipX = signal(0);

  tooltipY = signal(0);

  showRackTemplateModal = signal(false);

  // ── Floor layout (loaded per datacenter from the API) ────────────────────────
  readonly rackCells = signal<RackCell[]>([]);

  readonly dcStats = signal<DcStats>({
    rackCount: 0,
    deviceCount: 0,
    totalPowerKw: 0,
    capacityPct: 0,
  });

  // ── CRUD state ─────────────────────────────────────────────────────────────
  editForm = signal<Partial<DatacenterInfo> | null>(null);

  dcTier = signal<string>('3');

  dcStatus = signal<DatacenterStatus>('operational');

  deleteTarget = signal<DatacenterInfo | null>(null);

  // ── Validation feedback ──────────────────────────────────────────────────────
  readonly invalidFields = signal<Record<string, string>>({});

  readonly formErrorMessage = signal<string | null>(null);

  private readonly editSheetEl = viewChild<NativeElementRef>('editSheet');

  private readonly deleteModalEl = viewChild<NativeElementRef>('deleteModal');

  constructor() {
    effect(() => {
      const el = this.editSheetEl()?.nativeElement;
      if (this.editForm() !== null) el?.show?.();
      else el?.hide?.();
    });
    effect(() => {
      const el = this.deleteModalEl()?.nativeElement;
      if (this.deleteTarget() !== null) el?.show?.();
      else el?.hide?.();
    });
  }

  ngOnInit(): void {
    firstValueFrom(this.dcApi.listSites())
      .then((res) => {
        this.mutableDcs.set(res.sites.map((s) => DatacenterApiService.mapSite(s)));
        if (!this.selectedDcId()) {
          const first = this.mutableDcs()[0]?.id ?? '';
          if (first) this.selectDc(first);
        }
      })
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));
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
    'bg-emerald-50 border-emerald-300 text-emerald-700 hover:border-emerald-500 cursor-pointer';

  readonly rackFillBarClass = (): string => 'bg-emerald-200';

  readonly statusBadgeClass = (status: DatacenterStatus): string => {
    switch (status) {
      case 'operational':
        return 'bg-teal-50 text-teal-700 ring-1 ring-teal-200';
      case 'degraded':
        return 'bg-amber-50 text-amber-700 ring-1 ring-amber-200';
      case 'maintenance':
        return 'bg-slate-100 text-slate-500 ring-1 ring-slate-200';
      default:
        return '';
    }
  };

  readonly statusDotClass = (status: DatacenterStatus): string => {
    switch (status) {
      case 'operational':
        return 'bg-teal-500';
      case 'degraded':
        return 'bg-amber-500';
      case 'maintenance':
        return 'bg-slate-400';
      default:
        return '';
    }
  };

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

  // ── CRUD form field refs ───────────────────────────────────────────────────
  private readonly fName = viewChild<NativeElementRef>('fName');

  private readonly fFullName = viewChild<NativeElementRef>('fFullName');

  private readonly fCity = viewChild<NativeElementRef>('fCity');

  private readonly fCountry = viewChild<NativeElementRef>('fCountry');

  private readonly fAddress = viewChild<NativeElementRef>('fAddress');

  private readonly fEstablished = viewChild<NativeElementRef>('fEstablished');

  private readonly fFloorSqm = viewChild<NativeElementRef>('fFloorSqm');

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

  openCreateDc(): void {
    this.clearErrors();
    this.editForm.set({
      id: '',
      name: '',
      fullName: '',
      city: '',
      country: '',
      address: '',
      tier: 3,
      established: new Date().getFullYear(),
      status: 'operational',
      floorSqm: 0,
    });
    this.dcTier.set('3');
    this.dcStatus.set('operational');
  }

  openEditDc(dc: DatacenterInfo): void {
    this.clearErrors();
    this.editForm.set({ ...dc });
    this.dcTier.set(String(dc.tier));
    this.dcStatus.set(dc.status);
  }

  closeEditForm(): void {
    this.clearErrors();
    this.editForm.set(null);
  }

  saveDc(): void {
    const form = this.editForm();
    if (!form) return;
    this.clearErrors();
    const updated: DatacenterInfo = {
      id: form.id || `dc-${Date.now()}`,
      name: this.fName()?.nativeElement.value ?? '',
      fullName: this.fFullName()?.nativeElement.value ?? '',
      city: this.fCity()?.nativeElement.value ?? '',
      country: this.fCountry()?.nativeElement.value ?? '',
      address: this.fAddress()?.nativeElement.value ?? '',
      tier: (parseInt(this.dcTier(), 10) || 3) as 1 | 2 | 3 | 4,
      status: this.dcStatus(),
      established: parseFloat(this.fEstablished()?.nativeElement.value ?? '0') || 0,
      floorSqm: parseFloat(this.fFloorSqm()?.nativeElement.value ?? '0') || 0,
      // Not modelled by the API.
      powerCapacityKw: 0,
      coolingCapacityKw: 0,
      pue: 0,
    };
    if (form.id) {
      firstValueFrom(this.dcApi.updateSite(updated))
        .then(() => {
          this.mutableDcs.update((list) => list.map((dc) => (dc.id === form.id ? updated : dc)));
          this.editForm.set(null);
        })
        .catch((err) => this.handleError(err));
    } else {
      firstValueFrom(this.dcApi.createSite(updated))
        .then((res) => {
          const created = { ...updated, id: res.siteId || updated.id };
          this.mutableDcs.update((list) => [...list, created]);
          this.selectDc(created.id);
          this.editForm.set(null);
        })
        .catch((err) => this.handleError(err));
    }
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
          this.selectDc(remaining[0]?.id ?? '');
        }
        this.deleteTarget.set(null);
      })
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));
  }

  // ── Actions ────────────────────────────────────────────────────────────────

  selectDc(id: string): void {
    this.selectedDcId.set(id);
    this.hoveredRackId.set(null);
    this.loadFloor(id).catch(() => undefined);
  }

  onMapMouseMove(event: MouseEvent): void {
    this.tooltipX.set(event.clientX + 20);
    this.tooltipY.set(event.clientY + 20);
  }

  navigateToRack(rackId: string): void {
    this.router.navigate(['/racks', rackId]);
  }

  readonly formatPowerKw = (kw: number): string => `${kw.toFixed(1)} kW`;
}
