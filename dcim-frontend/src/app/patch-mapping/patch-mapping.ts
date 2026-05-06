import {
  ChangeDetectionStrategy,
  Component,
  CUSTOM_ELEMENTS_SCHEMA,
  effect,
  ElementRef,
  inject,
  signal,
  computed,
  viewChild,
} from '@angular/core';
import { Router } from '@angular/router';
import PatchMappingFlowWrapperComponent from './patch-mapping-flow-wrapper';
import CableListComponent from './cable-list/cable-list';
import CableFormComponent from './cable-form/cable-form';
import { Cable, DEVICE_PORTS, MOCK_CABLES } from './cable.model';

@Component({
  selector: 'app-patch-mapping',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [PatchMappingFlowWrapperComponent, CableListComponent, CableFormComponent],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  host: {
    class: 'flex flex-col bg-white text-slate-900',
    '[class.min-h-screen]': "activeView() === 'list'",
    '[class.overflow-hidden]': "activeView() === 'topology'",
    '[style.height]': "activeView() === 'topology' ? 'calc(100dvh - 4.25rem)' : null",
  },
  templateUrl: './patch-mapping.html',
})
export default class PatchMappingComponent {
  private readonly router = inject(Router);

  readonly selectedDcId = signal('ams-01');

  readonly activeView = signal<'list' | 'topology'>('list');

  // ── Cable state ────────────────────────────────────────────────────────────
  readonly mutableCables = signal([...MOCK_CABLES]);

  readonly dcCables = computed(() =>
    this.mutableCables().filter((c) => c.dcId === this.selectedDcId()),
  );

  readonly editCable = signal<Partial<Cable> | null>(null);

  readonly deleteCable = signal<Cable | null>(null);

  readonly DEVICE_PORTS = DEVICE_PORTS;

  private readonly cableSheetEl = viewChild<ElementRef>('cableSheet');

  private readonly deleteModalEl = viewChild<ElementRef>('deleteModal');

  constructor() {
    effect(() => {
      const el = this.cableSheetEl()?.nativeElement as { show?: () => void; hide?: () => void };
      if (this.editCable() !== null) el?.show?.();
      else el?.hide?.();
    });
    effect(() => {
      const el = this.deleteModalEl()?.nativeElement as { show?: () => void; hide?: () => void };
      if (this.deleteCable() !== null) el?.show?.();
      else el?.hide?.();
    });
  }

  // ── CRUD actions ───────────────────────────────────────────────────────────

  openAddCable(): void {
    this.editCable.set({ dcId: this.selectedDcId(), status: 'connected' });
  }

  openEditCable(cable: Cable): void {
    this.editCable.set({ ...cable });
  }

  openEditCableById(id: string): void {
    const cable = this.mutableCables().find((c) => c.id === id);
    if (cable) this.openEditCable(cable);
  }

  saveFromForm(cable: Cable): void {
    if (cable.id) {
      this.mutableCables.update((list) => list.map((c) => (c.id === cable.id ? cable : c)));
    } else {
      const id = `cab-${Date.now().toString(36)}`;
      this.mutableCables.update((list) => [...list, { ...cable, id, dcId: this.selectedDcId() }]);
    }
    this.editCable.set(null);
  }

  closeForm(): void {
    this.editCable.set(null);
  }

  openDeleteCable(cable: Cable): void {
    this.deleteCable.set(cable);
    this.editCable.set(null);
  }

  cancelDelete(): void {
    this.deleteCable.set(null);
  }

  confirmDelete(): void {
    const target = this.deleteCable();
    if (!target) return;
    this.mutableCables.update((list) => list.filter((c) => c.id !== target.id));
    this.deleteCable.set(null);
  }

  navigateToDevice(id: string): void {
    this.router.navigate(['/racks/device', id]);
  }

  // ── CSV export ─────────────────────────────────────────────────────────────

  exportCsv(): void {
    const cables = this.dcCables();
    const headers = [
      'ID',
      'Label',
      'A Device',
      'A Port',
      'A Port Type',
      'B Device',
      'B Port',
      'B Port Type',
      'Status',
      'Type',
      'Color',
      'Description',
      'Comments',
    ];
    const rows = cables.map((c) => [
      c.id,
      c.label ?? '',
      c.aSide.deviceName,
      c.aSide.portName,
      c.aSide.portType,
      c.bSide.deviceName,
      c.bSide.portName,
      c.bSide.portType,
      c.status,
      c.type,
      c.color ?? '',
      c.description ?? '',
      c.comments ?? '',
    ]);
    const csvContent = [headers, ...rows]
      .map((row) => row.map((cell) => `"${String(cell).replace(/"/g, '""')}"`).join(','))
      .join('\r\n');
    const blob = new Blob([csvContent], { type: 'text/csv;charset=utf-8;' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `cables-${this.selectedDcId()}-${new Date().toISOString().slice(0, 10)}.csv`;
    a.click();
    URL.revokeObjectURL(url);
  }
}
