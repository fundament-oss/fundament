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
import { takeUntilDestroyed, toObservable } from '@angular/core/rxjs-interop';
import { FormsModule } from '@angular/forms';
import { RouterLink } from '@angular/router';
import { debounce, distinctUntilChanged, firstValueFrom, skip, timer } from 'rxjs';
import { AssetCategory, CatalogEntry } from '../inventory/inventory';
import CatalogApiService from './catalog-api.service';
import InventoryApiService from '../inventory/inventory-api.service';
import connectErrorMessage from '../../connect/error';
import parseValidationError from '../../connect/validation';
import { AssetStatus as ProtoStatus } from '../../generated/v1/common_pb';
import type { Asset as ProtoAsset } from '../../generated/v1/asset_pb';
import DropdownSyncDirective from '../shared/dropdown-sync.directive';

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
  imports: [RouterLink, FormsModule, DropdownSyncDirective],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  host: { class: 'flex flex-col min-h-screen bg-white' },
})
export default class CatalogComponent implements OnInit {
  private readonly catalogApi = inject(CatalogApiService);

  private readonly inventoryApi = inject(InventoryApiService);

  /** All assets, used to derive instance counts per catalog entry. */
  private readonly assets = signal<ProtoAsset[]>([]);

  searchQuery = signal('');

  categoryFilter = signal<AssetCategory | 'all'>('all');

  readonly categories: AssetCategory[] = [
    'Server',
    'Switch',
    'Storage',
    'Power',
    'Firewall',
    'Cooling',
    'KVM',
    'Memory',
    'Disk',
    'NIC',
    'PSU',
    'CPU',
    'GPU',
    'Transceiver',
    'Other',
  ];

  // ── Mutable catalog list ───────────────────────────────────────────────────
  readonly mutableCatalog = signal<CatalogEntry[]>([]);

  // ── CRUD state ─────────────────────────────────────────────────────────────
  editEntry = signal<Partial<CatalogEntry> | null>(null);

  entryCategory = signal<AssetCategory>('Server');

  entryErrorMessage = signal<string | null>(null);

  invalidFields = signal<InvalidFields>({});

  deleteEntry = signal<CatalogEntry | null>(null);

  specRows = signal<{ key: string; value: string }[]>([]);

  private readonly entrySheetEl = viewChild<NativeElementRef>('entrySheet');

  private readonly entryModalEl = viewChild<NativeElementRef>('entryModal');

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
    effect(() => {
      const el = this.entryModalEl()?.nativeElement;
      if (this.deleteEntry() !== null) el?.show?.();
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

  readonly totalProducts = computed(() => this.allRows().length);

  readonly totalAssets = computed(() => this.allRows().reduce((s, r) => s + r.total, 0));

  readonly totalAvailable = computed(() => this.allRows().reduce((s, r) => s + r.available, 0));

  readonly totalIssues = computed(() => this.allRows().reduce((s, r) => s + r.issues, 0));

  readonly categoryCounts = computed(() => {
    const counts: Record<string, number> = {};
    this.allRows().forEach((row) => {
      counts[row.entry.category] = (counts[row.entry.category] ?? 0) + 1;
    });
    return counts;
  });

  selectCategory(cat: AssetCategory | 'all'): void {
    this.categoryFilter.set(cat);
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

  openEditEntry(entry: CatalogEntry, event: Event): void {
    event.preventDefault();
    event.stopPropagation();
    this.clearEntryErrors();
    this.editEntry.set({ ...entry });
    this.entryCategory.set(entry.category);
    this.specRows.set(Object.entries(entry.specs).map(([key, value]) => ({ key, value })));
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

  openDeleteEntry(entry: CatalogEntry, event: Event): void {
    event.preventDefault();
    event.stopPropagation();
    this.deleteEntry.set(entry);
  }

  cancelDeleteEntry(): void {
    this.deleteEntry.set(null);
  }

  confirmDeleteEntry(): void {
    const target = this.deleteEntry();
    if (!target) return;
    firstValueFrom(this.catalogApi.deleteCatalogEntry(target.id))
      .then(() => {
        this.mutableCatalog.update((list) => list.filter((e) => e.id !== target.id));
        this.deleteEntry.set(null);
      })
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));
  }

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

  readonly categoryBadgeClass = (category: AssetCategory): string => {
    const map: Partial<Record<AssetCategory, string>> = {
      Server: 'bg-indigo-50 text-indigo-700',
      Switch: 'bg-violet-50 text-violet-700',
      Storage: 'bg-blue-50 text-blue-700',
      Power: 'bg-amber-50 text-amber-700',
      Firewall: 'bg-red-50 text-red-700',
      Cooling: 'bg-cyan-50 text-cyan-700',
      KVM: 'bg-slate-100 text-slate-600',
      Memory: 'bg-emerald-50 text-emerald-700',
      Disk: 'bg-orange-50 text-orange-700',
      NIC: 'bg-teal-50 text-teal-700',
      PSU: 'bg-yellow-50 text-yellow-700',
      CPU: 'bg-purple-50 text-purple-700',
      GPU: 'bg-pink-50 text-pink-700',
      Transceiver: 'bg-sky-50 text-sky-700',
      Other: 'bg-slate-100 text-slate-600',
    };
    return map[category] ?? 'bg-slate-100 text-slate-600';
  };
}
