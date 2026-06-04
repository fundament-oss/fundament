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
  viewChild,
} from '@angular/core';
import { takeUntilDestroyed, toObservable } from '@angular/core/rxjs-interop';
import { RouterLink } from '@angular/router';
import { debounce, distinctUntilChanged, firstValueFrom, skip, timer } from 'rxjs';
import type { AssetStats } from '../../generated/v1/asset_pb';
import { RackSlotType } from '../../generated/v1/common_pb';
import InventoryApiService from './inventory-api.service';
import CatalogApiService from '../catalog/catalog-api.service';
import PlacementApiService, { RackOption } from './placement-api.service';
import connectErrorMessage from '../../connect/error';
import parseValidationError from '../../connect/validation';
import DropdownSyncDirective from '../shared/dropdown-sync.directive';

export type AssetStatus =
  | 'needs-repair'
  | 'decommissioned'
  | 'deployed'
  | 'available'
  | 'on-order'
  | 'requested';

export type AssetCategory =
  | 'Server'
  | 'Switch'
  | 'Storage'
  | 'Power'
  | 'Firewall'
  | 'Cooling'
  | 'KVM'
  | 'Other'
  | 'Memory'
  | 'Disk'
  | 'NIC'
  | 'PSU'
  | 'CPU'
  | 'GPU'
  | 'Transceiver';

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

@Component({
  selector: 'app-inventory',
  templateUrl: './inventory.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [RouterLink, DropdownSyncDirective],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  host: {
    class: 'flex flex-col min-h-screen bg-white',
  },
})
export default class InventoryComponent implements OnInit {
  private readonly inventoryApi = inject(InventoryApiService);

  private readonly catalogApi = inject(CatalogApiService);

  private readonly placementApi = inject(PlacementApiService);

  readonly assets = signal<Asset[]>([]);

  readonly catalog = signal<CatalogEntry[]>([]);

  private catalogById = new Map<string, CatalogEntry>();

  readonly stats = signal<AssetStats | null>(null);

  searchQuery = signal('');

  statusFilter = signal<AssetStatus | 'all'>('all');

  categoryFilter = signal<AssetCategory | 'all'>('all');

  sortDirection = signal<'asc' | 'desc'>('asc');

  // ── CRUD state ─────────────────────────────────────────────────────────────
  editAsset = signal<Partial<Asset> | null>(null);

  /** Rack placement of the asset being edited; null when adding or unplaced. */
  editPlacement = signal<{
    id: string;
    rackId: string;
    unit: number;
    slotType: RackSlotType;
  } | null>(null);

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

  readonly slotTypes: { value: RackSlotType; label: string }[] = [
    { value: RackSlotType.UNIT, label: 'Unit' },
    { value: RackSlotType.POWER, label: 'Power' },
    { value: RackSlotType.ZERO_U, label: 'Zero-U' },
  ];

  deleteAsset = signal<Asset | null>(null);

  // ── Validation feedback ──────────────────────────────────────────────────────
  readonly invalidFields = signal<Record<string, string>>({});

  readonly formErrorMessage = signal<string | null>(null);

  private readonly assetSheetEl = viewChild<ElementRef>('assetSheet');

  private readonly assetModalEl = viewChild<ElementRef>('assetModal');

  private readonly fAssetDevice = viewChild<ElementRef>('fAssetDevice');

  private readonly fAssetTag = viewChild<ElementRef>('fAssetTag');

  private readonly fAssetStatus = viewChild<ElementRef>('fAssetStatus');

  private readonly fAssetSerial = viewChild<ElementRef>('fAssetSerial');

  private readonly fAssetWarranty = viewChild<ElementRef>('fAssetWarranty');

  private readonly fAssetRack = viewChild<ElementRef>('fAssetRack');

  private readonly fAssetRackUnit = viewChild<ElementRef>('fAssetRackUnit');

  private readonly fAssetSlotType = viewChild<ElementRef>('fAssetSlotType');

  private readonly fAssetNotes = viewChild<ElementRef>('fAssetNotes');

  constructor() {
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
    effect(() => {
      const el = this.assetModalEl()?.nativeElement as { show?: () => void; hide?: () => void };
      if (this.deleteAsset() !== null) el?.show?.();
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

  readonly categories: AssetCategory[] = [
    'Server',
    'Switch',
    'Storage',
    'Power',
    'Firewall',
    'Cooling',
    'KVM',
    'Other',
    'Memory',
    'Disk',
    'NIC',
    'PSU',
    'CPU',
    'GPU',
    'Transceiver',
  ];

  readonly statuses: { value: AssetStatus; label: string }[] = [
    { value: 'deployed', label: 'Deployed' },
    { value: 'available', label: 'Available' },
    { value: 'on-order', label: 'On Order' },
    { value: 'requested', label: 'Requested' },
    { value: 'needs-repair', label: 'Needs Repair' },
    { value: 'decommissioned', label: 'Decommissioned' },
  ];

  private loadAssets(): void {
    firstValueFrom(
      this.inventoryApi.listAssets({
        search: this.searchQuery().trim(),
        status: this.statusFilter(),
        category: this.categoryFilter(),
        sortDirection: this.sortDirection(),
      }),
    )
      .then((res) =>
        this.assets.set(res.assets.map((a) => InventoryApiService.mapAsset(a, this.catalogById))),
      )
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));
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

  readonly totalCount = computed(() => this.stats()?.total ?? 0);

  readonly deployedCount = computed(() => this.stats()?.deployed ?? 0);

  readonly availableCount = computed(() => this.stats()?.available ?? 0);

  readonly issuesCount = computed(() => {
    const s = this.stats();
    return s ? s.needsRepair + s.decommissioned : 0;
  });

  // ── Filter / sort actions ──────────────────────────────────────────────────

  selectStatus(status: AssetStatus | 'all'): void {
    this.statusFilter.set(status);
    this.reload();
  }

  selectCategory(category: AssetCategory | 'all'): void {
    this.categoryFilter.set(category);
    this.reload();
  }

  toggleSort(): void {
    this.sortDirection.update((d) => (d === 'asc' ? 'desc' : 'asc'));
    this.reload();
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

  openCreateAsset(): void {
    this.clearErrors();
    this.editPlacement.set(null);
    this.editAsset.set({
      id: '',
      deviceCatalogId: this.catalog()[0]?.id ?? '',
      assetTag: '',
      status: 'available',
      notes: '',
    });
  }

  openEditAsset(asset: Asset, event: Event): void {
    event.preventDefault();
    event.stopPropagation();
    this.clearErrors();
    // Resolve the existing placement before opening, so the location picker
    // renders with the right rack pre-selected.
    firstValueFrom(this.placementApi.getPlacementByAsset(asset.id))
      .then((res) => {
        const p = res.placement;
        this.editPlacement.set(
          p && p.location.case === 'rack'
            ? {
                id: p.id,
                rackId: p.location.value.rackId,
                unit: p.location.value.rackUnitStart,
                slotType: p.location.value.rackSlotType,
              }
            : null,
        );
      })
      .catch((err) => {
        this.editPlacement.set(null);
        // eslint-disable-next-line no-console
        console.error(connectErrorMessage(err));
      })
      .finally(() => this.editAsset.set({ ...asset }));
  }

  closeAssetForm(): void {
    this.clearErrors();
    this.editAsset.set(null);
  }

  saveAsset(): void {
    const form = this.editAsset();
    if (!form) return;
    this.clearErrors();
    const deviceCatalogId =
      (this.fAssetDevice()?.nativeElement as HTMLSelectElement)?.value ??
      form.deviceCatalogId ??
      '';
    const entry = this.catalogById.get(deviceCatalogId);
    const warranty = (this.fAssetWarranty()?.nativeElement as HTMLInputElement)?.value ?? '';
    const updated: Asset = {
      id: form.id ?? '',
      deviceCatalogId,
      model: entry?.model ?? form.model ?? 'Unknown device',
      category: entry?.category ?? form.category ?? 'Other',
      assetTag: (this.fAssetTag()?.nativeElement as HTMLInputElement)?.value ?? '',
      status: ((this.fAssetStatus()?.nativeElement as HTMLSelectElement)?.value ??
        'available') as AssetStatus,
      serialNumber: (this.fAssetSerial()?.nativeElement as HTMLInputElement)?.value ?? '',
      warrantyExpiry: warranty || undefined,
      notes: (this.fAssetNotes()?.nativeElement as HTMLInputElement)?.value ?? '',
    };
    if (form.id) {
      firstValueFrom(this.inventoryApi.updateAsset(updated))
        .then(() => this.reconcilePlacement(updated.id))
        .then(() => {
          this.assets.update((list) => list.map((a) => (a.id === form.id ? updated : a)));
          this.loadStats();
          this.editAsset.set(null);
        })
        .catch((err) => this.handleError(err));
    } else {
      firstValueFrom(this.inventoryApi.createAsset(updated))
        .then((res) =>
          this.reconcilePlacement(res.assetId).then(() => {
            this.assets.update((list) => [{ ...updated, id: res.assetId }, ...list]);
            this.loadStats();
            this.editAsset.set(null);
          }),
        )
        .catch((err) => this.handleError(err));
    }
  }

  private reconcilePlacement(assetId: string): Promise<unknown> {
    const rackId = (this.fAssetRack()?.nativeElement as HTMLSelectElement)?.value ?? '';
    const unit =
      parseInt((this.fAssetRackUnit()?.nativeElement as HTMLInputElement)?.value ?? '', 10) || 0;
    const slotType =
      (Number(
        (this.fAssetSlotType()?.nativeElement as HTMLSelectElement)?.value,
      ) as RackSlotType) || RackSlotType.UNIT;
    return this.placementApi.reconcilePlacement({
      assetId,
      rackId,
      unit,
      slotType,
      existingPlacementId: this.editPlacement()?.id ?? null,
    });
  }

  openDeleteAsset(asset: Asset, event: Event): void {
    event.preventDefault();
    event.stopPropagation();
    this.deleteAsset.set(asset);
  }

  cancelDeleteAsset(): void {
    this.deleteAsset.set(null);
  }

  confirmDeleteAsset(): void {
    const target = this.deleteAsset();
    if (!target) return;
    firstValueFrom(this.inventoryApi.deleteAsset(target.id))
      .then(() => {
        this.assets.update((list) => list.filter((a) => a.id !== target.id));
        this.loadStats();
        this.deleteAsset.set(null);
      })
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));
  }

  statusLabel(status: AssetStatus): string {
    return this.statuses.find((s) => s.value === status)?.label ?? status;
  }

  readonly statusBadgeClass = (status: AssetStatus): string => {
    const map: Record<AssetStatus, string> = {
      'needs-repair': 'bg-amber-50 text-amber-700',
      decommissioned: 'bg-red-50 text-red-600',
      deployed: 'bg-teal-50 text-teal-700',
      available: 'bg-green-50 text-green-700',
      'on-order': 'bg-indigo-50 text-indigo-600',
      requested: 'bg-slate-100 text-slate-600',
    };
    return map[status];
  };

  readonly statusDotClass = (status: AssetStatus): string => {
    const map: Record<AssetStatus, string> = {
      'needs-repair': 'bg-amber-400',
      decommissioned: 'bg-red-400',
      deployed: 'bg-teal-400',
      available: 'bg-green-400',
      'on-order': 'bg-indigo-400',
      requested: 'bg-slate-400',
    };
    return map[status];
  };

  readonly categoryIcon = (category: AssetCategory): string => {
    const map: Partial<Record<AssetCategory, string>> = {
      Server: 'cylinder-split',
      Switch: 'list',
      Storage: 'rectangle-stack',
      Power: 'lock-closed',
      Firewall: 'shield-check-mark',
      Cooling: 'cloud',
      KVM: 'puzzle-piece',
      Other: 'ellipsis',
      Memory: 'folder',
      Disk: 'cylinder-split',
      NIC: 'puzzle-piece',
      PSU: 'lock-closed',
      CPU: 'gear',
      GPU: 'gear',
      Transceiver: 'puzzle-piece',
    };
    return map[category] ?? 'rectangle-stack';
  };
}
