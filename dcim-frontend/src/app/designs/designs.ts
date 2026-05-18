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
import { LogicalDesign, LogicalDesignStatus } from './design.model';
import DesignApiService from './design-api.service';
import connectErrorMessage from '../../connect/error';

interface NativeElementRef {
  nativeElement: { value: string; show?: () => void; hide?: () => void };
}

@Component({
  selector: 'app-designs',
  templateUrl: './designs.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [RouterLink],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  host: { class: 'flex flex-col min-h-screen bg-white' },
})
export default class DesignsComponent implements OnInit {
  private readonly designApi = inject(DesignApiService);

  statusFilter = signal<LogicalDesignStatus | 'all'>('all');

  searchQuery = signal('');

  // ── Mutable designs list ───────────────────────────────────────────────────
  readonly mutableDesigns = signal<LogicalDesign[]>([]);

  // ── CRUD state — null = closed, object = open ──────────────────────────────
  editDesign = signal<Partial<LogicalDesign> | null>(null);

  deleteDesign = signal<LogicalDesign | null>(null);

  private readonly designSheetEl = viewChild<NativeElementRef>('designSheet');

  private readonly deleteModalEl = viewChild<NativeElementRef>('deleteModal');

  private readonly fDesignName = viewChild<NativeElementRef>('fDesignName');

  constructor() {
    effect(() => {
      const el = this.designSheetEl()?.nativeElement;
      if (this.editDesign() !== null) el?.show?.();
      else el?.hide?.();
    });
    effect(() => {
      const el = this.deleteModalEl()?.nativeElement;
      if (this.deleteDesign() !== null) el?.show?.();
      else el?.hide?.();
    });
  }

  ngOnInit(): void {
    firstValueFrom(this.designApi.listDesigns())
      .then((res) => this.mutableDesigns.set(res.designs.map((d) => DesignApiService.mapDesign(d))))
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));
  }

  readonly filtered = computed(() => {
    const q = this.searchQuery().toLowerCase();
    const status = this.statusFilter();
    return this.mutableDesigns().filter((d) => {
      if (status !== 'all' && d.status !== status) return false;
      if (q && !d.name.toLowerCase().includes(q)) return false;
      return true;
    });
  });

  readonly counts = computed(() => {
    const all = this.mutableDesigns();
    return {
      all: all.length,
      draft: all.filter((d) => d.status === 'draft').length,
      active: all.filter((d) => d.status === 'active').length,
      archived: all.filter((d) => d.status === 'archived').length,
    };
  });

  // ── Actions ────────────────────────────────────────────────────────────────

  openNewDesign(): void {
    this.editDesign.set({ id: '', name: '', version: 1, status: 'draft' });
  }

  closeDesignForm(): void {
    this.editDesign.set(null);
  }

  saveDesign(): void {
    const name = this.fDesignName()?.nativeElement.value ?? '';
    if (!name?.trim()) return;
    firstValueFrom(this.designApi.createDesign(name.trim()))
      .then((res) => {
        const design: LogicalDesign = {
          id: res.designId,
          name: name.trim(),
          version: 1,
          status: 'draft',
          created: new Date().toISOString(),
        };
        this.mutableDesigns.update((list) => [design, ...list]);
        this.editDesign.set(null);
      })
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));
  }

  archiveDesign(design: LogicalDesign): void {
    firstValueFrom(this.designApi.updateDesign(design.id, 'archived'))
      .then(() =>
        this.mutableDesigns.update((list) =>
          list.map((d) =>
            d.id === design.id ? { ...d, status: 'archived' as LogicalDesignStatus } : d,
          ),
        ),
      )
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));
  }

  openDeleteDesign(design: LogicalDesign): void {
    this.deleteDesign.set(design);
  }

  cancelDeleteDesign(): void {
    this.deleteDesign.set(null);
  }

  confirmDeleteDesign(): void {
    const target = this.deleteDesign();
    if (!target) return;
    firstValueFrom(this.designApi.deleteDesign(target.id))
      .then(() => {
        this.mutableDesigns.update((list) => list.filter((d) => d.id !== target.id));
        this.deleteDesign.set(null);
      })
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));
  }

  readonly statusBadgeClass = (status: LogicalDesignStatus): string => {
    const statusMap: Record<LogicalDesignStatus, string> = {
      draft: 'bg-slate-100 text-slate-600',
      active: 'bg-green-50 text-green-700',
      archived: 'bg-amber-50 text-amber-700',
    };
    return statusMap[status];
  };

  readonly statusLabel = (status: LogicalDesignStatus): string => {
    const statusMap: Record<LogicalDesignStatus, string> = {
      draft: 'Draft',
      active: 'Active',
      archived: 'Archived',
    };
    return statusMap[status];
  };

  readonly formatDate = (dateStr: string): string =>
    new Date(dateStr).toLocaleDateString('en-GB', {
      day: 'numeric',
      month: 'short',
      year: 'numeric',
    });
}
