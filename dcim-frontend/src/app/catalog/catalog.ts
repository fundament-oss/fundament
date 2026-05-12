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
import { RouterLink } from '@angular/router';
import { firstValueFrom } from 'rxjs';
import { AssetCategory, CatalogEntry, MOCK_ASSETS } from '../inventory/inventory';
import CatalogApiService from './catalog-api.service';
import connectErrorMessage from '../../connect/error';

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

@Component({
  selector: 'app-catalog',
  templateUrl: './catalog.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [RouterLink],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  host: { class: 'flex flex-col min-h-screen bg-white' },
})
export default class CatalogComponent implements OnInit {
  private readonly catalogApi = inject(CatalogApiService);

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

  deleteEntry = signal<CatalogEntry | null>(null);

  specRows = signal<{ key: string; value: string }[]>([]);

  private readonly entrySheetEl = viewChild<NativeElementRef>('entrySheet');

  private readonly entryModalEl = viewChild<NativeElementRef>('entryModal');

  private readonly fEntryModel = viewChild<NativeElementRef>('fEntryModel');

  private readonly fEntryMfr = viewChild<NativeElementRef>('fEntryMfr');

  private readonly fEntryCat = viewChild<NativeElementRef>('fEntryCat');

  constructor() {
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
    firstValueFrom(this.catalogApi.listCatalog())
      .then((res) =>
        this.mutableCatalog.set(
          res.entries.map((s) => CatalogApiService.mapCatalogEntry(s.entry!)),
        ),
      )
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));
  }

  private readonly allRows = computed<CatalogRow[]>(() =>
    this.mutableCatalog().map((entry) => {
      const assets = MOCK_ASSETS.filter((a) => a.model === entry.model);
      return {
        entry,
        total: assets.length,
        deployed: assets.filter((a) => a.status === 'deployed').length,
        available: assets.filter((a) => a.status === 'available').length,
        issues: assets.filter((a) => a.status === 'needs-repair' || a.status === 'decommissioned')
          .length,
      };
    }),
  );

  readonly rows = computed<CatalogRow[]>(() => {
    const q = this.searchQuery().toLowerCase();
    const cat = this.categoryFilter();
    return this.allRows().filter((row) => {
      if (cat !== 'all' && row.entry.category !== cat) return false;
      if (
        q &&
        !row.entry.model.toLowerCase().includes(q) &&
        !row.entry.manufacturer.toLowerCase().includes(q)
      )
        return false;
      return true;
    });
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
    this.editEntry.set({ id: '', model: '', manufacturer: '', category: 'Server', specs: {} });
    this.specRows.set([{ key: '', value: '' }]);
  }

  openEditEntry(entry: CatalogEntry, event: Event): void {
    event.preventDefault();
    event.stopPropagation();
    this.editEntry.set({ ...entry });
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
    const model = this.fEntryModel()?.nativeElement.value ?? '';
    const manufacturer = this.fEntryMfr()?.nativeElement.value ?? '';
    const category = (this.fEntryCat()?.nativeElement.value ?? 'Server') as AssetCategory;
    const specs: Record<string, string> = {};
    this.specRows().forEach((row) => {
      if (row.key.trim()) specs[row.key.trim()] = row.value;
    });
    const entry: CatalogEntry = { id: form.id || '', model, manufacturer, category, specs };
    if (form.id) {
      firstValueFrom(this.catalogApi.updateCatalogEntry(entry))
        .then(() => {
          this.mutableCatalog.update((list) => list.map((e) => (e.id === form.id ? entry : e)));
          this.editEntry.set(null);
        })
        // eslint-disable-next-line no-console
        .catch((err) => console.error(connectErrorMessage(err)));
    } else {
      firstValueFrom(this.catalogApi.createCatalogEntry(entry))
        .then((res) => {
          this.mutableCatalog.update((list) => [...list, { ...entry, id: res.catalogEntryId }]);
          this.editEntry.set(null);
        })
        // eslint-disable-next-line no-console
        .catch((err) => console.error(connectErrorMessage(err)));
    }
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
