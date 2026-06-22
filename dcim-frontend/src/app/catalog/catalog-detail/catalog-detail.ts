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
} from '@angular/core';
import { FormsModule } from '@angular/forms';
import { RouterLink, ActivatedRoute } from '@angular/router';
import { firstValueFrom } from 'rxjs';
import {
  Asset,
  AssetCategory,
  AssetStatus,
  CatalogEntry,
  PortDefinition,
  PortCompatibility,
} from '../../inventory/inventory';
import {
  ASSET_STATUS_BADGE_CLASS,
  ASSET_STATUS_DOT_CLASS,
  ASSET_STATUS_LABEL,
} from '../../inventory/asset-status';
import CatalogApiService from '../catalog-api.service';
import InventoryApiService from '../../inventory/inventory-api.service';
import connectErrorMessage from '../../../connect/error';
import parseValidationError from '../../../connect/validation';
import type { Asset as ProtoAsset } from '../../../generated/v1/asset_pb';
import DropdownSyncDirective from '../../shared/dropdown-sync.directive';

interface NativeElementRef {
  nativeElement: { value: string; show?: () => void; hide?: () => void };
}

@Component({
  selector: 'app-catalog-detail',
  templateUrl: './catalog-detail.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [RouterLink, FormsModule, DropdownSyncDirective],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  host: { class: 'block bg-slate-50 dark:bg-gray-900 min-h-screen' },
})
export default class CatalogDetailComponent implements OnInit {
  private readonly route = inject(ActivatedRoute);

  private readonly catalogApi = inject(CatalogApiService);

  private readonly inventoryApi = inject(InventoryApiService);

  readonly catalogId = computed(() => this.route.snapshot.paramMap.get('id') ?? '');

  readonly entry = signal<CatalogEntry | undefined>(undefined);

  readonly entryLoaded = signal(false);

  /** Raw assets from the API; instances of this entry are derived in `assets`. */
  private readonly rawAssets = signal<ProtoAsset[]>([]);

  readonly assets = computed<Asset[]>(() => {
    const id = this.catalogId();
    const entry = this.entry();
    const catalog = entry ? new Map([[entry.id, entry]]) : new Map<string, CatalogEntry>();
    return this.rawAssets()
      .filter((a) => a.deviceCatalogId === id)
      .map((a) => InventoryApiService.mapAsset(a, catalog));
  });

  readonly deployedCount = computed(
    () => this.assets().filter((a) => a.status === 'deployed').length,
  );

  readonly availableCount = computed(
    () => this.assets().filter((a) => a.status === 'available').length,
  );

  readonly issuesCount = computed(
    () =>
      this.assets().filter((a) => a.status === 'needs-repair' || a.status === 'decommissioned')
        .length,
  );

  // ── Port definitions ───────────────────────────────────────────────────────
  readonly mutablePortDefs = signal<PortDefinition[]>([]);

  readonly mutableCompatibilities = signal<PortCompatibility[]>([]);

  readonly portDefs = computed(() =>
    this.mutablePortDefs().filter((p) => p.catalogEntryId === this.catalogId()),
  );

  readonly compatibilities = computed(() => {
    const pdIds = new Set(this.portDefs().map((p) => p.id));
    return this.mutableCompatibilities().filter((c) => pdIds.has(c.portDefinitionId));
  });

  // ── Port definition CRUD state ────────────────────────────────────────────
  editPortDef = signal<Partial<PortDefinition> | null>(null);

  portType = signal<string>('');

  portDirection = signal<string>('bidir');

  deletePortDef = signal<PortDefinition | null>(null);

  // ── Validation feedback (shared by the port + compatibility forms) ────────────
  readonly invalidFields = signal<Record<string, string>>({});

  readonly formErrorMessage = signal<string | null>(null);

  /** Selectable port-type enum values (proto-aligned keys + display labels). */
  readonly PORT_TYPES: { value: string; label: string }[] = [
    { value: 'network', label: 'Network' },
    { value: 'power_in', label: 'Power in' },
    { value: 'power_out', label: 'Power out' },
    { value: 'slot', label: 'Slot' },
    { value: 'bay', label: 'Bay' },
    { value: 'console', label: 'Console' },
  ];

  portTypeLabel(value: string): string {
    return this.PORT_TYPES.find((t) => t.value === value)?.label ?? value;
  }

  /** Selectable port-direction enum values (proto-aligned keys + labels). */
  readonly PORT_DIRECTIONS: { value: string; label: string }[] = [
    { value: 'bidir', label: 'Bidirectional' },
    { value: 'in', label: 'In' },
    { value: 'out', label: 'Out' },
  ];

  private readonly portSheetEl = viewChild<NativeElementRef>('portSheet');

  private readonly portModalEl = viewChild<NativeElementRef>('portModal');

  private readonly fPortName = viewChild<NativeElementRef>('fPortName');

  private readonly fPortMedia = viewChild<NativeElementRef>('fPortMedia');

  private readonly fPortSpeed = viewChild<NativeElementRef>('fPortSpeed');

  private readonly fPortPower = viewChild<NativeElementRef>('fPortPower');

  // ── Port compatibility CRUD state ─────────────────────────────────────────
  addCompatPortDefId = signal<string | null>(null);

  /** Catalog entry selected in the "Add compatibility" sheet (empty = none). */
  readonly compatEntryId = signal('');

  deleteCompat = signal<PortCompatibility | null>(null);

  private readonly compatSheetEl = viewChild<NativeElementRef>('compatSheet');

  private readonly compatDeleteModalEl = viewChild<NativeElementRef>('compatDeleteModal');

  // ── Catalog list for compatibility dropdown ────────────────────────────────
  readonly allCatalogEntries = signal<CatalogEntry[]>([]);

  /**
   * Catalog entries selectable in the picker: every entry except this device
   * itself and the ones already marked compatible with the active port.
   */
  readonly availableCompatEntries = computed<CatalogEntry[]>(() => {
    const pdId = this.addCompatPortDefId();
    const taken = new Set(
      this.mutableCompatibilities()
        .filter((c) => c.portDefinitionId === pdId)
        .map((c) => c.compatibleCatalogEntryId),
    );
    return this.allCatalogEntries().filter((e) => e.id !== this.catalogId() && !taken.has(e.id));
  });

  constructor() {
    effect(() => {
      const el = this.portSheetEl()?.nativeElement;
      if (this.editPortDef() !== null) el?.show?.();
      else el?.hide?.();
    });
    effect(() => {
      const el = this.portModalEl()?.nativeElement;
      if (this.deletePortDef() !== null) el?.show?.();
      else el?.hide?.();
    });
    effect(() => {
      const el = this.compatSheetEl()?.nativeElement;
      if (this.addCompatPortDefId() !== null) el?.show?.();
      else el?.hide?.();
    });
    effect(() => {
      const el = this.compatDeleteModalEl()?.nativeElement;
      if (this.deleteCompat() !== null) el?.show?.();
      else el?.hide?.();
    });
  }

  ngOnInit(): void {
    firstValueFrom(this.catalogApi.getCatalogEntry(this.catalogId()))
      .then((res) => {
        if (res.entry) this.entry.set(CatalogApiService.mapCatalogEntry(res.entry));
      })
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)))
      .finally(() => this.entryLoaded.set(true));

    firstValueFrom(this.catalogApi.listPortDefinitions(this.catalogId()))
      .then((res) => {
        const portDefs = res.portDefinitions.map((p) => CatalogApiService.mapPortDefinition(p));
        this.mutablePortDefs.set(portDefs);
        // Load each port's existing compatibilities so the "Compatible with"
        // chips render on initial page load, not just after opening the picker.
        return Promise.all(
          portDefs.map((pd) =>
            firstValueFrom(this.catalogApi.listPortCompatibilities(pd.id)).then((compatRes) =>
              compatRes.compatibilities.map((c) => CatalogApiService.mapPortCompatibility(c)),
            ),
          ),
        );
      })
      .then((compatArrays) => this.mutableCompatibilities.set(compatArrays.flat()))
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));

    firstValueFrom(
      this.inventoryApi.listAssets({ status: 'all', category: 'all', sortDirection: 'asc' }),
    )
      .then((res) => this.rawAssets.set(res.assets))
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));

    // Full catalog, for the port-compatibility picker and name resolution.
    firstValueFrom(this.catalogApi.listCatalog())
      .then((res) =>
        this.allCatalogEntries.set(
          res.entries.map((s) => CatalogApiService.mapCatalogEntry(s.entry!)),
        ),
      )
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));
  }

  // ── Port definition actions ────────────────────────────────────────────────

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

  openCreatePortDef(): void {
    this.clearErrors();
    this.editPortDef.set({
      id: '',
      catalogEntryId: this.catalogId(),
      name: '',
      portType: '',
      direction: 'bidir',
    });
    this.portType.set('');
    this.portDirection.set('bidir');
  }

  openEditPortDef(pd: PortDefinition): void {
    this.clearErrors();
    this.editPortDef.set({ ...pd });
    this.portType.set(pd.portType);
    this.portDirection.set(pd.direction ?? 'bidir');
  }

  closePortDefForm(): void {
    this.clearErrors();
    this.editPortDef.set(null);
  }

  savePortDef(): void {
    const form = this.editPortDef();
    if (!form) return;
    this.clearErrors();
    const name = this.fPortName()?.nativeElement.value ?? '';
    const portType = this.portType();
    const direction = this.portDirection();
    const mediaType = this.fPortMedia()?.nativeElement.value ?? '';
    const speedRaw = this.fPortSpeed()?.nativeElement.value;
    const powerRaw = this.fPortPower()?.nativeElement.value;
    const speedGbps = speedRaw ? parseFloat(speedRaw) : undefined;
    const powerWatts = powerRaw ? parseFloat(powerRaw) : undefined;
    const pd: PortDefinition = {
      id: form.id || '',
      catalogEntryId: this.catalogId(),
      name,
      portType,
      direction,
      ordinal: form.ordinal ?? this.portDefs().length,
      ...(mediaType ? { mediaType } : {}),
      ...(speedGbps != null && !Number.isNaN(speedGbps) ? { speedGbps } : {}),
      ...(powerWatts != null && !Number.isNaN(powerWatts) ? { powerWatts } : {}),
    };
    if (form.id) {
      firstValueFrom(this.catalogApi.updatePortDefinition(pd))
        .then(() => {
          this.mutablePortDefs.update((list) => list.map((p) => (p.id === form.id ? pd : p)));
          this.editPortDef.set(null);
        })
        .catch((err) => this.handleError(err));
    } else {
      firstValueFrom(this.catalogApi.createPortDefinition(pd))
        .then((res) => {
          this.mutablePortDefs.update((list) => [...list, { ...pd, id: res.portDefinitionId }]);
          this.editPortDef.set(null);
        })
        .catch((err) => this.handleError(err));
    }
  }

  openDeletePortDef(pd: PortDefinition): void {
    this.deletePortDef.set(pd);
  }

  cancelDeletePortDef(): void {
    this.deletePortDef.set(null);
  }

  confirmDeletePortDef(): void {
    const target = this.deletePortDef();
    if (!target) return;
    firstValueFrom(this.catalogApi.deletePortDefinition(target.id))
      .then(() => {
        this.mutablePortDefs.update((list) => list.filter((p) => p.id !== target.id));
        this.mutableCompatibilities.update((list) =>
          list.filter((c) => c.portDefinitionId !== target.id),
        );
        this.deletePortDef.set(null);
      })
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));
  }

  // ── Port compatibility actions ─────────────────────────────────────────────

  openAddCompatibility(portDefId: string): void {
    this.clearErrors();
    this.compatEntryId.set('');
    this.addCompatPortDefId.set(portDefId);
  }

  cancelAddCompatibility(): void {
    this.clearErrors();
    this.compatEntryId.set('');
    this.addCompatPortDefId.set(null);
  }

  confirmAddCompatibility(): void {
    const pdId = this.addCompatPortDefId();
    const entryId = this.compatEntryId();
    if (!pdId || !entryId) return;
    this.clearErrors();
    const entry = this.allCatalogEntries().find((e) => e.id === entryId);
    firstValueFrom(this.catalogApi.createPortCompatibility(pdId, entryId))
      .then(() => {
        const created: PortCompatibility = {
          id: `${pdId}:${entryId}`,
          portDefinitionId: pdId,
          compatibleCategory: entry?.category ?? 'Other',
          compatibleCatalogEntryId: entryId,
        };
        this.mutableCompatibilities.update((list) => [...list, created]);
        this.compatEntryId.set('');
        this.addCompatPortDefId.set(null);
      })
      .catch((err) => this.handleError(err));
  }

  openDeleteCompat(compat: PortCompatibility): void {
    this.deleteCompat.set(compat);
  }

  cancelDeleteCompat(): void {
    this.deleteCompat.set(null);
  }

  confirmDeleteCompat(): void {
    const target = this.deleteCompat();
    if (!target) return;
    firstValueFrom(
      this.catalogApi.deletePortCompatibility(
        target.portDefinitionId,
        target.compatibleCatalogEntryId,
      ),
    )
      .then(() => {
        this.mutableCompatibilities.update((list) => list.filter((c) => c.id !== target.id));
        this.deleteCompat.set(null);
      })
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));
  }

  readonly compatibleEntryName = (entryId: string): string =>
    this.allCatalogEntries().find((e) => e.id === entryId)?.model ?? entryId;

  /**
   * Chip label for a compatibility: the specific model when narrowed to one
   * catalog entry, otherwise the whole accepted category (e.g. "Any Server").
   */
  readonly compatLabel = (compat: PortCompatibility): string =>
    compat.compatibleCatalogEntryId
      ? this.compatibleEntryName(compat.compatibleCatalogEntryId)
      : `Any ${compat.compatibleCategory}`;

  portDefName(pdId: string): string {
    return this.mutablePortDefs().find((p) => p.id === pdId)?.name ?? pdId;
  }

  compatibilitiesForPortDef(pdId: string): PortCompatibility[] {
    return this.mutableCompatibilities().filter((c) => c.portDefinitionId === pdId);
  }

  readonly specEntries = (specs: Record<string, string>): { key: string; value: string }[] =>
    Object.entries(specs).map(([key, value]) => ({ key, value }));

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

  readonly statusLabel = (status: AssetStatus): string => ASSET_STATUS_LABEL[status];

  readonly statusBadgeClass = (status: AssetStatus): string => ASSET_STATUS_BADGE_CLASS[status];

  readonly statusDotClass = (status: AssetStatus): string => ASSET_STATUS_DOT_CLASS[status];
}
