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
import { RouterLink, ActivatedRoute } from '@angular/router';
import { firstValueFrom } from 'rxjs';
import {
  Asset,
  AssetCategory,
  AssetStatus,
  CatalogEntry,
  MOCK_ASSETS,
  MOCK_CATALOG,
  PortDefinition,
  PortCompatibility,
} from '../../inventory/inventory';
import CatalogApiService from '../catalog-api.service';
import connectErrorMessage from '../../../connect/error';

interface NativeElementRef {
  nativeElement: { value: string; show?: () => void; hide?: () => void };
}

@Component({
  selector: 'app-catalog-detail',
  templateUrl: './catalog-detail.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [RouterLink],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  host: { class: 'block bg-slate-50 min-h-screen' },
})
export default class CatalogDetailComponent implements OnInit {
  private readonly route = inject(ActivatedRoute);

  private readonly catalogApi = inject(CatalogApiService);

  readonly catalogId = computed(() => this.route.snapshot.paramMap.get('id') ?? '');

  readonly entry = computed<CatalogEntry | undefined>(() =>
    MOCK_CATALOG.find((e) => e.id === this.catalogId()),
  );

  readonly assets = computed<Asset[]>(() => {
    const model = this.entry()?.model;
    return model ? MOCK_ASSETS.filter((a) => a.model === model) : [];
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

  deletePortDef = signal<PortDefinition | null>(null);

  private readonly portSheetEl = viewChild<NativeElementRef>('portSheet');

  private readonly portModalEl = viewChild<NativeElementRef>('portModal');

  private readonly fPortName = viewChild<NativeElementRef>('fPortName');

  private readonly fPortType = viewChild<NativeElementRef>('fPortType');

  private readonly fPortSpeed = viewChild<NativeElementRef>('fPortSpeed');

  private readonly fPortPower = viewChild<NativeElementRef>('fPortPower');

  // ── Port compatibility CRUD state ─────────────────────────────────────────
  addCompatPortDefId = signal<string | null>(null);

  deleteCompat = signal<PortCompatibility | null>(null);

  private readonly compatModalEl = viewChild<NativeElementRef>('compatModal');

  private readonly compatDeleteModalEl = viewChild<NativeElementRef>('compatDeleteModal');

  private readonly fCompatEntry = viewChild<NativeElementRef>('fCompatEntry');

  // ── Catalog list for compatibility dropdown ────────────────────────────────
  readonly allCatalogEntries = MOCK_CATALOG;

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
      const el = this.compatModalEl()?.nativeElement;
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
    firstValueFrom(this.catalogApi.listPortDefinitions(this.catalogId()))
      .then((res) =>
        this.mutablePortDefs.set(
          res.portDefinitions.map((p) => CatalogApiService.mapPortDefinition(p)),
        ),
      )
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));
  }

  // ── Port definition actions ────────────────────────────────────────────────

  openCreatePortDef(): void {
    this.editPortDef.set({ id: '', catalogEntryId: this.catalogId(), name: '', portType: '' });
  }

  openEditPortDef(pd: PortDefinition): void {
    this.editPortDef.set({ ...pd });
  }

  closePortDefForm(): void {
    this.editPortDef.set(null);
  }

  savePortDef(): void {
    const form = this.editPortDef();
    if (!form) return;
    const name = this.fPortName()?.nativeElement.value ?? '';
    const portType = this.fPortType()?.nativeElement.value ?? '';
    const speedRaw = this.fPortSpeed()?.nativeElement.value;
    const powerRaw = this.fPortPower()?.nativeElement.value;
    const speedGbps = speedRaw ? parseFloat(speedRaw) : undefined;
    const powerWatts = powerRaw ? parseFloat(powerRaw) : undefined;
    const pd: PortDefinition = {
      id: form.id || '',
      catalogEntryId: this.catalogId(),
      name,
      portType,
      ...(speedGbps != null && !Number.isNaN(speedGbps) ? { speedGbps } : {}),
      ...(powerWatts != null && !Number.isNaN(powerWatts) ? { powerWatts } : {}),
    };
    if (form.id) {
      firstValueFrom(this.catalogApi.updatePortDefinition(pd))
        .then(() => {
          this.mutablePortDefs.update((list) => list.map((p) => (p.id === form.id ? pd : p)));
          this.editPortDef.set(null);
        })
        // eslint-disable-next-line no-console
        .catch((err) => console.error(connectErrorMessage(err)));
    } else {
      firstValueFrom(this.catalogApi.createPortDefinition(pd))
        .then((res) => {
          this.mutablePortDefs.update((list) => [...list, { ...pd, id: res.portDefinitionId }]);
          this.editPortDef.set(null);
        })
        // eslint-disable-next-line no-console
        .catch((err) => console.error(connectErrorMessage(err)));
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
    this.addCompatPortDefId.set(portDefId);
    firstValueFrom(this.catalogApi.listPortCompatibilities(portDefId))
      .then((res) => {
        const existing = new Set(
          this.mutableCompatibilities().map(
            (c) => `${c.portDefinitionId}:${c.compatibleCatalogEntryId}`,
          ),
        );
        const newOnes = res.compatibilities
          .map((c) => CatalogApiService.mapPortCompatibility(c))
          .filter((c) => !existing.has(`${c.portDefinitionId}:${c.compatibleCatalogEntryId}`));
        if (newOnes.length) {
          this.mutableCompatibilities.update((list) => [...list, ...newOnes]);
        }
      })
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));
  }

  cancelAddCompatibility(): void {
    this.addCompatPortDefId.set(null);
  }

  confirmAddCompatibility(): void {
    const pdId = this.addCompatPortDefId();
    const entryId = this.fCompatEntry()?.nativeElement.value ?? '';
    if (!pdId || !entryId) return;
    firstValueFrom(this.catalogApi.createPortCompatibility(pdId, entryId))
      .then(() => {
        const created: PortCompatibility = {
          id: `${pdId}:${entryId}`,
          portDefinitionId: pdId,
          compatibleCatalogEntryId: entryId,
        };
        this.mutableCompatibilities.update((list) => [...list, created]);
        this.addCompatPortDefId.set(null);
      })
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));
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
    MOCK_CATALOG.find((e) => e.id === entryId)?.model ?? entryId;

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
      deployed: 'bg-teal-400',
      available: 'bg-green-400',
      'needs-repair': 'bg-amber-400',
      decommissioned: 'bg-slate-300',
      'on-order': 'bg-blue-400',
      requested: 'bg-purple-400',
    };
    return classes[status];
  };
}
