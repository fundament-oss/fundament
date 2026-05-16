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
import ShoppingListComponent from './shopping-list/shopping-list';
import { Cable, CableSide, CableStatus, CableType, DEVICE_PORTS, MOCK_CABLES } from './cable.model';
import { DATACENTER_INFO } from '../datacenters/datacenter.model';
import { RACKS } from '../racks/rack.model';

@Component({
  selector: 'app-patch-mapping',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [
    PatchMappingFlowWrapperComponent,
    CableListComponent,
    CableFormComponent,
    ShoppingListComponent,
  ],
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

  // ── Shopping list state ────────────────────────────────────────────────────
  readonly shoppingListOpen = signal(false);

  readonly plannedCables = computed(() => this.dcCables().filter((c) => c.status === 'planned'));

  readonly plannedCount = computed(() => this.plannedCables().length);

  readonly selectedDcLabel = computed(
    () => DATACENTER_INFO.find((dc) => dc.id === this.selectedDcId())?.name ?? this.selectedDcId(),
  );

  // ── Topology filters ───────────────────────────────────────────────────────
  readonly topologyStatusFilter = signal<CableStatus | ''>('');

  readonly topologyTypeFilter = signal<CableType | ''>('');

  readonly DEVICE_PORTS = DEVICE_PORTS;

  readonly CABLE_TYPES: CableType[] = [
    'cat5e',
    'cat6',
    'cat6a',
    'cat7',
    'cat8',
    'dac',
    'aoc',
    'mmf',
    'smf',
    'power',
    'console',
    'usb',
    'other',
  ];

  readonly CABLE_STATUSES: { value: CableStatus; label: string }[] = [
    { value: 'planned', label: 'Planned' },
    { value: 'connected', label: 'Connected' },
    { value: 'decommissioned', label: 'Decommissioned' },
  ];

  private readonly cableSheetEl = viewChild<ElementRef>('cableSheet');

  private readonly deleteModalEl = viewChild<ElementRef>('deleteModal');

  private readonly shoppingSheetEl = viewChild<ElementRef>('shoppingSheet');

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
    effect(() => {
      const el = this.shoppingSheetEl()?.nativeElement as { show?: () => void; hide?: () => void };
      if (this.shoppingListOpen()) el?.show?.();
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

  openAddCableFromConnection(conn: {
    sourceDeviceId: string;
    sourcePortId: string;
    targetDeviceId: string;
    targetPortId: string;
  }): void {
    const allDevices = RACKS.flatMap((r) => r.devices);
    const aDevice = allDevices.find((d) => d.id === conn.sourceDeviceId);
    const bDevice = allDevices.find((d) => d.id === conn.targetDeviceId);
    const aPort = (DEVICE_PORTS[conn.sourceDeviceId] ?? []).find((p) => p.id === conn.sourcePortId);
    const bPort = (DEVICE_PORTS[conn.targetDeviceId] ?? []).find((p) => p.id === conn.targetPortId);

    if (!aDevice || !bDevice || !aPort || !bPort) {
      this.openAddCable();
      return;
    }

    const aSide: CableSide = {
      deviceId: conn.sourceDeviceId,
      deviceName: aDevice.name,
      portId: conn.sourcePortId,
      portName: aPort.name,
      portType: aPort.type,
    };
    const bSide: CableSide = {
      deviceId: conn.targetDeviceId,
      deviceName: bDevice.name,
      portId: conn.targetPortId,
      portName: bPort.name,
      portType: bPort.type,
    };

    this.editCable.set({
      dcId: this.selectedDcId(),
      status: 'connected',
      aSide,
      bSide,
    });
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
}
