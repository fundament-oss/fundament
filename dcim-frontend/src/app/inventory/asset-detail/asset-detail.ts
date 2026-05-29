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
import { RouterLink, ActivatedRoute, Router } from '@angular/router';
import { firstValueFrom } from 'rxjs';
import { RackSlotType } from '../../../generated/v1/common_pb';
import {
  Asset,
  AssetCategory,
  AssetStatus,
  CatalogEntry,
  HistoryEntry,
  MOCK_ASSETS,
  NoteComment,
} from '../inventory';
import InventoryApiService from '../inventory-api.service';
import CatalogApiService from '../../catalog/catalog-api.service';
import NoteApiService from '../note-api.service';
import PlacementApiService, { RackOption } from '../placement-api.service';
import connectErrorMessage from '../../../connect/error';

interface AssetExtraDetail {
  serial: string;
  manufacturer: string;
  purchaseDate: string;
  purchaseCost: string;
  warrantyExpires: string;
  supportContract: string;
}

const MOCK_EXTRA_DETAILS: Record<string, AssetExtraDetail> = {
  'AST-001': {
    serial: 'SN-DELL-R750-00A12X',
    manufacturer: 'Dell Technologies',
    purchaseDate: '2024-03-15',
    purchaseCost: '€ 18.450',
    warrantyExpires: '2027-03-15',
    supportContract: 'ProSupport Plus 3yr',
  },
  'AST-002': {
    serial: 'SN-CSC-9300-B05YZ',
    manufacturer: 'Cisco Systems',
    purchaseDate: '2023-11-20',
    purchaseCost: '€ 9.200',
    warrantyExpires: '2026-11-20',
    supportContract: 'SmartNet 3yr',
  },
  'AST-003': {
    serial: 'SN-NTAP-A800-C08AB',
    manufacturer: 'NetApp',
    purchaseDate: '2025-01-08',
    purchaseCost: '€ 124.000',
    warrantyExpires: '2028-01-08',
    supportContract: 'SupportEdge Premium 3yr',
  },
  'AST-004': {
    serial: 'SN-HPE-DL380-D14CC',
    manufacturer: 'Hewlett Packard Enterprise',
    purchaseDate: '2022-07-10',
    purchaseCost: '€ 14.700',
    warrantyExpires: '2025-07-10',
    supportContract: 'HPE Foundation Care 3yr',
  },
  'AST-007': {
    serial: 'SN-DELL-R650-A13QR',
    manufacturer: 'Dell Technologies',
    purchaseDate: '2024-06-01',
    purchaseCost: '€ 11.800',
    warrantyExpires: '2027-06-01',
    supportContract: 'ProSupport Plus 3yr',
  },
  'AST-008': {
    serial: 'SN-PA-5250-F01MN',
    manufacturer: 'Palo Alto Networks',
    purchaseDate: '2023-09-05',
    purchaseCost: '€ 42.000',
    warrantyExpires: '2026-09-05',
    supportContract: 'Premium Support 3yr',
  },
  'AST-009': {
    serial: 'SN-PURE-X70-C04KL',
    manufacturer: 'Pure Storage',
    purchaseDate: '2024-01-22',
    purchaseCost: '€ 87.500',
    warrantyExpires: '2027-01-22',
    supportContract: 'Evergreen//One',
  },
  'AST-012': {
    serial: 'SN-ARIS-7050-B01PQ',
    manufacturer: 'Arista Networks',
    purchaseDate: '2023-04-14',
    purchaseCost: '€ 31.200',
    warrantyExpires: '2026-04-14',
    supportContract: 'Arista TAC 3yr',
  },
  'AST-013': {
    serial: 'SN-LNV-SR650-A05RR',
    manufacturer: 'Lenovo',
    purchaseDate: '2021-12-03',
    purchaseCost: '€ 12.600',
    warrantyExpires: '2024-12-03',
    supportContract: 'Foundation Service 3yr',
  },
  'AST-018': {
    serial: 'SN-FTN-FG600-F02ST',
    manufacturer: 'Fortinet',
    purchaseDate: '2023-08-17',
    purchaseCost: '€ 28.900',
    warrantyExpires: '2026-08-17',
    supportContract: 'FortiCare 360 3yr',
  },
};

@Component({
  selector: 'app-asset-detail',
  templateUrl: './asset-detail.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [RouterLink],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  host: { class: 'block bg-slate-50 min-h-screen' },
})
export default class AssetDetailComponent implements OnInit {
  private readonly route = inject(ActivatedRoute);

  private readonly router = inject(Router);

  private readonly inventoryApi = inject(InventoryApiService);

  private readonly catalogApi = inject(CatalogApiService);

  private readonly noteApi = inject(NoteApiService);

  private readonly placementApi = inject(PlacementApiService);

  readonly assetId = computed(() => this.route.snapshot.paramMap.get('id') ?? '');

  readonly asset = signal<Asset | undefined>(undefined);

  /** False until the API call settles, so "not found" only shows after loading. */
  readonly assetLoaded = signal(false);

  readonly catalogEntry = signal<CatalogEntry | undefined>(undefined);

  /** Resolved physical location; undefined until loaded, or when the asset is unplaced. */
  readonly assetLocation = signal<
    { datacenter: string; rack: string; rackUnit: number; slotType: RackSlotType } | undefined
  >(undefined);

  readonly parentAsset = computed<Asset | undefined>(() => {
    const parentId = this.asset()?.parentId;
    return parentId ? MOCK_ASSETS.find((a) => a.id === parentId) : undefined;
  });

  readonly childAssets = computed<Asset[]>(() =>
    MOCK_ASSETS.filter((a) => a.parentId === this.assetId()),
  );

  readonly assetHistory = signal<HistoryEntry[]>([]);

  // ── Edit asset ─────────────────────────────────────────────────────────────

  /** Holds the asset being edited; non-null while the edit sheet is open. */
  readonly editAsset = signal<Partial<Asset> | null>(null);

  readonly statuses: { value: AssetStatus; label: string }[] = [
    { value: 'deployed', label: 'Deployed' },
    { value: 'available', label: 'Available' },
    { value: 'on-order', label: 'On Order' },
    { value: 'requested', label: 'Requested' },
    { value: 'needs-repair', label: 'Needs Repair' },
    { value: 'decommissioned', label: 'Decommissioned' },
  ];

  /** Rack placement of the asset being edited; null while loading or when unplaced. */
  readonly editPlacement = signal<{
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

  readonly defaultSlotType = RackSlotType.UNIT;

  readonly slotTypeLabel = (slotType: RackSlotType): string =>
    this.slotTypes.find((s) => s.value === slotType)?.label ?? '—';

  private readonly assetSheetEl = viewChild<ElementRef>('assetSheet');

  private readonly fAssetTag = viewChild<ElementRef>('fAssetTag');

  private readonly fAssetStatus = viewChild<ElementRef>('fAssetStatus');

  private readonly fAssetSerial = viewChild<ElementRef>('fAssetSerial');

  private readonly fAssetWarranty = viewChild<ElementRef>('fAssetWarranty');

  private readonly fAssetRack = viewChild<ElementRef>('fAssetRack');

  private readonly fAssetRackUnit = viewChild<ElementRef>('fAssetRackUnit');

  private readonly fAssetSlotType = viewChild<ElementRef>('fAssetSlotType');

  private readonly fAssetNotes = viewChild<ElementRef>('fAssetNotes');

  constructor() {
    effect(() => {
      const el = this.assetSheetEl()?.nativeElement as { show?: () => void; hide?: () => void };
      if (this.editAsset() !== null) el?.show?.();
      else el?.hide?.();
    });
  }

  ngOnInit(): void {
    this.loadAsset();
    this.loadHistory();
    this.loadNotes();
    this.loadLocation();
    this.loadRackOptions();
  }

  private loadRackOptions(): void {
    this.placementApi
      .listRackOptions()
      .then((racks) => this.racks.set(racks))
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));
  }

  private loadAsset(): void {
    firstValueFrom(this.inventoryApi.getAsset(this.assetId()))
      .then((res) => {
        const protoAsset = res.asset;
        if (!protoAsset) return undefined;
        if (!protoAsset.deviceCatalogId) {
          this.asset.set(InventoryApiService.mapAsset(protoAsset, new Map()));
          return undefined;
        }
        // Resolve the catalog entry so model, category and the specs panel populate.
        return firstValueFrom(this.catalogApi.getCatalogEntry(protoAsset.deviceCatalogId))
          .then((catRes) =>
            catRes.entry ? CatalogApiService.mapCatalogEntry(catRes.entry) : undefined,
          )
          .catch(() => undefined)
          .then((entry) => {
            const catalog = new Map<string, CatalogEntry>();
            if (entry) {
              catalog.set(entry.id, entry);
              this.catalogEntry.set(entry);
            }
            this.asset.set(InventoryApiService.mapAsset(protoAsset, catalog));
          });
      })
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)))
      .finally(() => this.assetLoaded.set(true));
  }

  private loadHistory(): void {
    firstValueFrom(this.inventoryApi.getAssetEvents(this.assetId()))
      .then((res) => this.assetHistory.set(res.events.map(InventoryApiService.mapAssetEvent)))
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));
  }

  private loadNotes(): void {
    firstValueFrom(this.noteApi.listNotesForAsset(this.assetId()))
      .then((res) => this.notes.set(res.notes.map(NoteApiService.mapNote)))
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));
  }

  private loadLocation(): void {
    firstValueFrom(this.inventoryApi.getAssetLocation(this.assetId()))
      .then((res) => {
        const loc = res.location;
        this.assetLocation.set(
          loc
            ? {
                datacenter: loc.siteName,
                rack: loc.rackName,
                rackUnit: loc.rackUnitStart,
                slotType: loc.rackSlotType,
              }
            : undefined,
        );
      })
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));
  }

  openEditAsset(): void {
    const current = this.asset();
    if (!current) return;
    // Resolve the existing placement before opening, so the location picker
    // renders with the right rack pre-selected.
    firstValueFrom(this.placementApi.getPlacementByAsset(current.id))
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
      .finally(() => this.editAsset.set({ ...current }));
  }

  closeAssetForm(): void {
    this.editAsset.set(null);
  }

  saveAsset(): void {
    const current = this.asset();
    if (!current) return;
    const warranty = (this.fAssetWarranty()?.nativeElement as HTMLInputElement)?.value ?? '';
    const updated: Asset = {
      ...current,
      assetTag: (this.fAssetTag()?.nativeElement as HTMLInputElement)?.value ?? current.assetTag,
      status: ((this.fAssetStatus()?.nativeElement as HTMLSelectElement)?.value ??
        current.status) as AssetStatus,
      serialNumber:
        (this.fAssetSerial()?.nativeElement as HTMLInputElement)?.value ??
        current.serialNumber ??
        '',
      warrantyExpiry: warranty || undefined,
      notes: (this.fAssetNotes()?.nativeElement as HTMLInputElement)?.value ?? current.notes,
    };
    firstValueFrom(this.inventoryApi.updateAsset(updated))
      .then(() => this.reconcilePlacement(updated.id))
      .then(() => {
        this.asset.set(updated);
        this.editAsset.set(null);
        this.loadLocation();
      })
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));
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

  readonly extraDetail = computed<AssetExtraDetail | undefined>(
    () => MOCK_EXTRA_DETAILS[this.assetId()],
  );

  readonly notes = signal<NoteComment[]>([]);

  readonly newNoteText = signal('');

  readonly statusLabel = (status: AssetStatus): string => {
    const labels: Record<AssetStatus, string> = {
      deployed: 'Deployed',
      available: 'Available',
      'needs-repair': 'Needs Repair',
      decommissioned: 'Decommissioned',
      'on-order': 'On Order',
      requested: 'Requested',
    };
    return labels[status];
  };

  readonly statusBadgeClass = (status: AssetStatus): string => {
    const classes: Record<AssetStatus, string> = {
      deployed: 'bg-teal-50 text-teal-700',
      available: 'bg-green-50 text-green-700',
      'needs-repair': 'bg-amber-50 text-amber-700',
      decommissioned: 'bg-slate-100 text-slate-500',
      'on-order': 'bg-blue-50 text-blue-700',
      requested: 'bg-purple-50 text-purple-700',
    };
    return classes[status];
  };

  readonly statusDotClass = (status: AssetStatus): string => {
    const classes: Record<AssetStatus, string> = {
      deployed: 'bg-teal-500',
      available: 'bg-green-500',
      'needs-repair': 'bg-amber-500',
      decommissioned: 'bg-slate-400',
      'on-order': 'bg-blue-500',
      requested: 'bg-purple-500',
    };
    return classes[status];
  };

  readonly statusIcon = (status: AssetStatus): string => {
    const icons: Record<AssetStatus, string> = {
      deployed: 'check-mark-circle',
      available: 'check-mark-circle',
      'needs-repair': 'exclamation-triangle',
      decommissioned: 'slash-circle',
      'on-order': 'arrow-right',
      requested: 'clock-arrow-counter-clockwise',
    };
    return icons[status];
  };

  readonly statusIconColor = (status: AssetStatus): string => {
    const colors: Record<AssetStatus, string> = {
      deployed: 'text-teal-500',
      available: 'text-green-500',
      'needs-repair': 'text-amber-500',
      decommissioned: 'text-slate-400',
      'on-order': 'text-blue-500',
      requested: 'text-purple-500',
    };
    return colors[status];
  };

  readonly statusIconBgClass = (status: AssetStatus): string => {
    const classes: Record<AssetStatus, string> = {
      deployed: 'bg-teal-50',
      available: 'bg-green-50',
      'needs-repair': 'bg-amber-50',
      decommissioned: 'bg-slate-100',
      'on-order': 'bg-blue-50',
      requested: 'bg-purple-50',
    };
    return `flex h-14 w-14 items-center justify-center rounded-full ${classes[status]}`;
  };

  readonly formatDaysAgo = (daysAgo: number): string => {
    if (daysAgo === 0) return 'Today';
    if (daysAgo === 1) return 'Yesterday';
    if (daysAgo < 30) return `${daysAgo} days ago`;
    const months = Math.floor(daysAgo / 30);
    return months === 1 ? '1 month ago' : `${months} months ago`;
  };

  readonly historyIcon = (action: HistoryEntry['action']): string => {
    const icons: Record<HistoryEntry['action'], string> = {
      received: 'arrow-right',
      deployed: 'check-mark-circle',
      moved: 'arrow-up-arrow-down',
      'repair-sent': 'gear',
      'repair-received': 'gear',
      decommissioned: 'slash-circle',
      requested: 'clock-arrow-counter-clockwise',
      note: 'info-circle',
    };
    return icons[action];
  };

  readonly historyIconBg = (action: HistoryEntry['action']): string => {
    const classes: Record<HistoryEntry['action'], string> = {
      received: 'bg-sky-50 text-sky-500',
      deployed: 'bg-teal-50 text-teal-500',
      moved: 'bg-sky-50 text-sky-500',
      'repair-sent': 'bg-amber-50 text-amber-500',
      'repair-received': 'bg-amber-50 text-amber-500',
      decommissioned: 'bg-slate-100 text-slate-500',
      requested: 'bg-purple-50 text-purple-500',
      note: 'bg-indigo-50 text-indigo-500',
    };
    return classes[action];
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
      Memory: 'folder-stack',
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
