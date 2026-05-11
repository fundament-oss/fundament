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
import { Router, RouterLink } from '@angular/router';
import { firstValueFrom } from 'rxjs';
import DcSelectorComponent from '../shared/dc-selector';
import DatacenterApiService from './datacenter-api.service';
import connectErrorMessage from '../../connect/error';
import { RACKS } from '../racks/rack.model';
import IsometricCanvasComponent from './isometric-canvas';
import {
  AisleDefinition,
  DATACENTER_INFO,
  DatacenterInfo,
  DatacenterStatus,
  FLOOR_CONFIGS,
  FLOOR_POSITIONS,
  RackCell,
  rackDeviceCount,
  rackFillPct,
  rackPowerW,
} from './datacenter.model';

interface NativeElementRef {
  nativeElement: { value: string; show?: () => void; hide?: () => void };
}

// ── Component ─────────────────────────────────────────────────────────────────

@Component({
  selector: 'app-datacenters',
  templateUrl: './datacenters.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [RouterLink, DcSelectorComponent, IsometricCanvasComponent],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  host: { class: 'flex flex-col bg-white text-slate-900' },
})
export default class DatacentersComponent implements OnInit {
  private readonly router = inject(Router);

  private readonly dcApi = inject(DatacenterApiService);

  // ── Mutable DC list ────────────────────────────────────────────────────────
  readonly mutableDcs = signal([...DATACENTER_INFO]);

  selectedDcId = signal('ams-01');

  viewMode = signal<'map' | 'isometric'>('map');

  hoveredRackId = signal<string | null>(null);

  tooltipX = signal(0);

  tooltipY = signal(0);

  showRackTemplateModal = signal(false);

  // ── CRUD state ─────────────────────────────────────────────────────────────
  editForm = signal<Partial<DatacenterInfo> | null>(null);

  deleteTarget = signal<DatacenterInfo | null>(null);

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
        const apiDcs = res.sites.map((s) => DatacenterApiService.mapSite(s));
        this.mutableDcs.update((list) =>
          list.map((dc) => {
            const api = apiDcs.find((a) => a.id === dc.id);
            return api ? { ...dc, name: api.name, address: api.address } : dc;
          }),
        );
      })
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));
  }

  readonly currentDc = computed(
    () => this.mutableDcs().find((dc) => dc.id === this.selectedDcId())!,
  );

  readonly dcRacks = computed(() => RACKS.filter((r) => r.dcId === this.selectedDcId()));

  readonly rackCells = computed((): RackCell[] =>
    FLOOR_POSITIONS.filter((p) => p.dcId === this.selectedDcId()).map((p) => {
      if (p.ownership === 'other-client') {
        return {
          rackId: undefined,
          rackName: '—',
          row: p.row,
          col: p.col,
          fillPct: 0,
          deviceCount: 0,
          powerW: 0,
          ownership: 'other-client' as const,
          floorStatus: 'n/a' as const,
        };
      }
      const id = p.rackId!;
      return {
        rackId: id,
        rackName: RACKS.find((r) => r.id === id)?.name ?? id,
        row: p.row,
        col: p.col,
        fillPct: rackFillPct(id),
        deviceCount: rackDeviceCount(id),
        powerW: rackPowerW(id),
        ownership: 'own' as const,
        floorStatus: p.floorStatus ?? 'operational',
      };
    }),
  );

  // All rows (including other-client) — used for the map view
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

  // Own racks only — used for the isometric view
  readonly ownFloorRows = computed((): Map<string, RackCell[]> => {
    const floorMap = new Map<string, RackCell[]>();
    this.rackCells()
      .filter((c) => c.ownership === 'own')
      .forEach((cell) => {
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

  readonly ownRows = computed(() => [...this.ownFloorRows().keys()].sort());

  readonly dcStats = computed(() => {
    const racks = this.dcRacks();
    const rackCount = racks.length;
    const deviceCount = racks.reduce((sum, r) => sum + r.devices.length, 0);
    const totalPowerW = racks.reduce(
      (sum, r) => sum + r.devices.reduce((s, d) => s + (d.ipmi?.averageW ?? 0), 0),
      0,
    );
    const totalUsedU = racks.reduce(
      (sum, r) => sum + r.devices.reduce((s, d) => s + d.uSize, 0),
      0,
    );
    const totalCapacity = racks.reduce((sum, r) => sum + r.totalU, 0);
    const capacityPct = totalCapacity > 0 ? Math.round((totalUsedU / totalCapacity) * 100) : 0;
    const issueCount = this.rackCells().filter((c) => c.floorStatus === 'issue').length;
    return { rackCount, deviceCount, totalPowerKw: totalPowerW / 1000, capacityPct, issueCount };
  });

  readonly hoveredCell = computed(() => {
    const id = this.hoveredRackId();
    return id ? (this.rackCells().find((c) => c.rackId === id) ?? null) : null;
  });

  readonly currentFloorConfig = computed(() =>
    FLOOR_CONFIGS.find((c) => c.dcId === this.selectedDcId()),
  );

  readonly firstRackRoute = computed(() => {
    const id = this.dcRacks()[0]?.id;
    return id ? ['/racks', id] : ['/racks'];
  });

  // ── Aisle helpers ──────────────────────────────────────────────────────────

  aisleAfterRow(row: string): AisleDefinition | undefined {
    return this.currentFloorConfig()?.aisles.find((a) => a.afterRow === row);
  }

  // ── Isometric SVG geometry ─────────────────────────────────────────────────

  readonly CELL_W = 56;

  readonly CELL_D = 32;

  readonly MAX_H = 90;

  readonly MIN_H = 20;

  readonly COL_GAP = 10;

  readonly ROW_GAP = 24;

  isoPoints(
    cell: RackCell,
    rowIndex: number,
  ): {
    top: string;
    left: string;
    right: string;
    label: { x: number; y: number };
    pctLabel: { x: number; y: number };
  } {
    const h = this.MIN_H + (cell.fillPct / 100) * (this.MAX_H - this.MIN_H);

    const originX =
      260 +
      (cell.col - 1) * (this.CELL_W / 2 + this.CELL_D / 2 + this.COL_GAP) -
      rowIndex * (this.CELL_W / 2 + this.COL_GAP);
    const originY =
      80 + rowIndex * (this.CELL_D / 2 + this.ROW_GAP) + (cell.col - 1) * (this.CELL_D / 2);

    const bx = originX;
    const by = originY + h;

    const t0x = bx;
    const t0y = by - h - this.CELL_D / 2;
    const t1x = bx + this.CELL_W / 2;
    const t1y = by - h;
    const t2x = bx;
    const t2y = by - h + this.CELL_D / 2;
    const t3x = bx - this.CELL_W / 2;
    const t3y = by - h;

    const l0x = bx - this.CELL_W / 2;
    const l0y = by - h;
    const l1x = bx;
    const l1y = by - h + this.CELL_D / 2;
    const l2x = bx;
    const l2y = by + this.CELL_D / 2;
    const l3x = bx - this.CELL_W / 2;
    const l3y = by;

    const r0x = bx;
    const r0y = by - h + this.CELL_D / 2;
    const r1x = bx + this.CELL_W / 2;
    const r1y = by - h;
    const r2x = bx + this.CELL_W / 2;
    const r2y = by;
    const r3x = bx;
    const r3y = by + this.CELL_D / 2;

    const pt = (...pairs: number[]) =>
      pairs
        .reduce<string[]>((acc, v, i) => {
          if (i % 2 === 0) acc.push(`${v}`);
          else acc[acc.length - 1] += `,${v}`;
          return acc;
        }, [])
        .join(' ');

    return {
      top: pt(t0x, t0y, t1x, t1y, t2x, t2y, t3x, t3y),
      left: pt(l0x, l0y, l1x, l1y, l2x, l2y, l3x, l3y),
      right: pt(r0x, r0y, r1x, r1y, r2x, r2y, r3x, r3y),
      label: { x: bx, y: by + this.CELL_D / 2 + 14 },
      pctLabel: { x: bx, y: t0y - 4 },
    };
  }

  readonly isoViewBox = computed((): string => {
    const rows = this.ownRows().length;
    const maxCols = Math.max(...this.ownRows().map((r) => this.ownFloorRows().get(r)!.length), 1);
    const w = 520 + maxCols * (this.CELL_W + this.COL_GAP);
    const h = 200 + rows * (this.MAX_H + this.CELL_D + this.ROW_GAP * 2);
    return `0 0 ${w} ${h}`;
  });

  // ── Color helpers ──────────────────────────────────────────────────────────

  readonly rackCellClass = (cell: RackCell): string => {
    if (cell.floorStatus === 'issue')
      return 'bg-red-50 border-red-300 text-red-600 hover:border-red-500 cursor-pointer';
    return 'bg-emerald-50 border-emerald-300 text-emerald-700 hover:border-emerald-500 cursor-pointer';
  };

  readonly rackFillBarClass = (cell: RackCell): string => {
    if (cell.floorStatus === 'issue') return 'bg-red-200';
    return 'bg-emerald-200';
  };

  static isoColorTop(cell: RackCell): string {
    return cell.floorStatus === 'issue' ? '#fca5a5' : '#6ee7b7';
  }

  static isoColorLeft(cell: RackCell): string {
    return cell.floorStatus === 'issue' ? '#f87171' : '#34d399';
  }

  static isoColorRight(cell: RackCell): string {
    return cell.floorStatus === 'issue' ? '#ef4444' : '#10b981';
  }

  static isoStroke(cell: RackCell, hovered: boolean): string {
    if (hovered) return '#6366f1';
    return cell.floorStatus === 'issue' ? '#fca5a5' : '#6ee7b7';
  }

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

  private readonly fTier = viewChild<NativeElementRef>('fTier');

  private readonly fStatus = viewChild<NativeElementRef>('fStatus');

  private readonly fEstablished = viewChild<NativeElementRef>('fEstablished');

  private readonly fPowerKw = viewChild<NativeElementRef>('fPowerKw');

  private readonly fCoolingKw = viewChild<NativeElementRef>('fCoolingKw');

  private readonly fFloorSqm = viewChild<NativeElementRef>('fFloorSqm');

  private readonly fPue = viewChild<NativeElementRef>('fPue');

  // ── CRUD actions ───────────────────────────────────────────────────────────

  openCreateDc(): void {
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
      powerCapacityKw: 0,
      coolingCapacityKw: 0,
      pue: 1.5,
    });
  }

  openEditDc(dc: DatacenterInfo): void {
    this.editForm.set({ ...dc });
  }

  closeEditForm(): void {
    this.editForm.set(null);
  }

  saveDc(): void {
    const form = this.editForm();
    if (!form) return;
    const updated: DatacenterInfo = {
      id: form.id || `dc-${Date.now()}`,
      name: this.fName()?.nativeElement.value ?? '',
      fullName: this.fFullName()?.nativeElement.value ?? '',
      city: this.fCity()?.nativeElement.value ?? '',
      country: this.fCountry()?.nativeElement.value ?? '',
      address: this.fAddress()?.nativeElement.value ?? '',
      tier: (parseInt(this.fTier()?.nativeElement.value ?? '3', 10) || 3) as 1 | 2 | 3 | 4,
      status: (this.fStatus()?.nativeElement.value ?? 'operational') as DatacenterStatus,
      established: parseFloat(this.fEstablished()?.nativeElement.value ?? '0') || 0,
      powerCapacityKw: parseFloat(this.fPowerKw()?.nativeElement.value ?? '0') || 0,
      coolingCapacityKw: parseFloat(this.fCoolingKw()?.nativeElement.value ?? '0') || 0,
      floorSqm: parseFloat(this.fFloorSqm()?.nativeElement.value ?? '0') || 0,
      pue: parseFloat(this.fPue()?.nativeElement.value ?? '0') || 0,
    };
    if (form.id) {
      firstValueFrom(this.dcApi.updateSite(form.id, updated.name, updated.address))
        .then(() => {
          this.mutableDcs.update((list) => list.map((dc) => (dc.id === form.id ? updated : dc)));
          this.editForm.set(null);
        })
        // eslint-disable-next-line no-console
        .catch((err) => console.error(connectErrorMessage(err)));
    } else {
      firstValueFrom(this.dcApi.createSite(updated.name, updated.address))
        .then((res) => {
          const created = { ...updated, id: res.siteId || updated.id };
          this.mutableDcs.update((list) => [...list, created]);
          this.selectedDcId.set(created.id);
          this.editForm.set(null);
        })
        // eslint-disable-next-line no-console
        .catch((err) => console.error(connectErrorMessage(err)));
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
          this.selectedDcId.set(remaining[0]?.id ?? '');
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
