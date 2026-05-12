import {
  ChangeDetectionStrategy,
  Component,
  computed,
  CUSTOM_ELEMENTS_SCHEMA,
  effect,
  inject,
  signal,
  viewChild,
} from '@angular/core';
import { toSignal } from '@angular/core/rxjs-interop';
import { ActivatedRoute, Router } from '@angular/router';
import { firstValueFrom, map } from 'rxjs';
import RackApiService from './rack-api.service';
import connectErrorMessage from '../../connect/error';
import DcSelectorComponent from '../shared/dc-selector';
import RackDiagramComponent from './rack-diagram/rack-diagram';
import RackDiagramEditorComponent from './rack-diagram-editor/rack-diagram-editor';
import { DeviceState, DeviceType, Rack, RackDevice, RACKS } from './rack.model';
import { DATACENTER_INFO, MOCK_RACK_ROWS } from '../datacenters/datacenter.model';

// ── Notes & History types ──────────────────────────────────────────────────────

interface RackNoteComment {
  author: string;
  initials: string;
  daysAgo: number;
  content: string;
}

interface RackNotes {
  description: string;
  comments: RackNoteComment[];
}

interface RackEvent {
  user: string;
  daysAgo: number;
  description: string;
  type: 'power' | 'hardware' | 'config' | 'alert';
}

// ── Mock data ─────────────────────────────────────────────────────────────────

const RACK_NOTES: Record<string, RackNotes> = {
  'ams-01-r01': {
    description:
      'Primary compute rack for alpha and beta teams. Power draw peaks at ~4 kW under full load. Scheduled for expansion in Q3.',
    comments: [
      {
        author: 'Jan de Vries',
        initials: 'JV',
        daysAgo: 2,
        content: 'Replaced faulty NIC on server-01. Back to green, monitoring for 48 h.',
      },
      {
        author: 'Sarah Müller',
        initials: 'SM',
        daysAgo: 9,
        content: 'Annual rack safety inspection completed. Certified OK until 2027-04.',
      },
      {
        author: 'Tom Bakker',
        initials: 'TB',
        daysAgo: 21,
        content: 'Added new patch panel in U3. Cable management updated and documented.',
      },
    ],
  },
  'ams-01-r02': {
    description:
      'Storage and backup rack. Houses the primary NAS and tape library. Keep ambient temperature below 22 °C.',
    comments: [
      {
        author: 'Tom Bakker',
        initials: 'TB',
        daysAgo: 5,
        content: 'Tape library firmware updated to v3.4.1. No issues observed.',
      },
      {
        author: 'Jan de Vries',
        initials: 'JV',
        daysAgo: 30,
        content: 'Replaced failed drive in NAS bay 7. Rebuild completed in 4 h.',
      },
    ],
  },
};

const RACK_HISTORY: Record<string, RackEvent[]> = {
  'ams-01-r01': [
    {
      user: 'Ops Team',
      daysAgo: 6,
      description: 'Rack powered on after scheduled maintenance window',
      type: 'power',
    },
    {
      user: 'Monitoring',
      daysAgo: 8,
      description: 'server-02 went offline — PSU fault detected',
      type: 'alert',
    },
    {
      user: 'Jan de Vries',
      daysAgo: 14,
      description: 'patch-panel-01 installed in U3',
      type: 'hardware',
    },
    {
      user: 'Automation',
      daysAgo: 27,
      description: 'Config push: VLAN 42 updated on tor-switch-01',
      type: 'config',
    },
    {
      user: 'Sarah Müller',
      daysAgo: 50,
      description: 'Firmware update applied to server-01 (BIOS 2.8.0)',
      type: 'hardware',
    },
  ],
  'ams-01-r02': [
    {
      user: 'Monitoring',
      daysAgo: 10,
      description: 'NAS reported degraded RAID — drive rebuild initiated',
      type: 'alert',
    },
    {
      user: 'Tom Bakker',
      daysAgo: 22,
      description: 'Tape library firmware updated to v3.4.1',
      type: 'hardware',
    },
    {
      user: 'Ops Team',
      daysAgo: 60,
      description: 'UPS bypass test performed — all systems nominal',
      type: 'power',
    },
  ],
};

// ── Helpers ───────────────────────────────────────────────────────────────────

function findFirstFreeSlot(rack: Rack, uSize: number): number | null {
  const occupied = new Set<number>();
  rack.devices.forEach((dev) => {
    for (let u = dev.uStart; u < dev.uStart + dev.uSize; u += 1) {
      occupied.add(u);
    }
  });
  for (let top = rack.totalU; top >= uSize; top -= 1) {
    let fits = true;
    for (let u = top; u > top - uSize; u -= 1) {
      if (occupied.has(u)) {
        fits = false;
        break;
      }
    }
    if (fits) return top - uSize + 1;
  }
  return null;
}

// ── NativeElementRef ──────────────────────────────────────────────────────────

interface NativeElementRef {
  nativeElement: { value: string; show?: () => void; hide?: () => void };
}

// ── Component ─────────────────────────────────────────────────────────────────

@Component({
  selector: 'app-racks',
  templateUrl: './racks.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [DcSelectorComponent, RackDiagramComponent, RackDiagramEditorComponent],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
})
export default class RacksComponent {
  private readonly rackApi = inject(RackApiService);

  private readonly route = inject(ActivatedRoute);

  private readonly router = inject(Router);

  readonly currentRackId = toSignal(this.route.paramMap.pipe(map((p) => p.get('rackId'))), {
    initialValue: this.route.snapshot.paramMap.get('rackId'),
  });

  viewMode = signal<'front' | 'back'>('front');

  searchQuery = signal('');

  activeModal = signal<'notes' | 'history' | null>(null);

  // ── Mutable rack list ──────────────────────────────────────────────────────
  readonly mutableRacks = signal([...RACKS]);

  // ── CRUD state ─────────────────────────────────────────────────────────────
  readonly editRack = signal<Partial<Rack> | null>(null);

  readonly deleteRack = signal<Rack | null>(null);

  // ── Edit-layout mode ───────────────────────────────────────────────────────
  readonly editMode = signal(false);

  readonly deleteDeviceTarget = signal<RackDevice | null>(null);

  readonly addDeviceForm = signal<Partial<RackDevice> | null>(null);

  readonly addDeviceSizeInput = signal(1);

  readonly addDeviceNameInput = signal('');

  readonly datacenters = DATACENTER_INFO;

  readonly rackRows = MOCK_RACK_ROWS;

  private readonly rackSheetEl = viewChild<NativeElementRef>('rackSheet');

  private readonly rackModalEl = viewChild<NativeElementRef>('rackModal');

  private readonly fRackName = viewChild<NativeElementRef>('fRackName');

  private readonly fRackDcId = viewChild<NativeElementRef>('fRackDcId');

  private readonly fRackTotalU = viewChild<NativeElementRef>('fRackTotalU');

  private readonly deviceSheetEl = viewChild<NativeElementRef>('deviceSheet');

  private readonly deviceModalEl = viewChild<NativeElementRef>('deviceModal');

  private readonly fDeviceName = viewChild<NativeElementRef>('fDeviceName');

  private readonly fDeviceType = viewChild<NativeElementRef>('fDeviceType');

  private readonly fDeviceUSize = viewChild<NativeElementRef>('fDeviceUSize');

  private readonly fDeviceState = viewChild<NativeElementRef>('fDeviceState');

  readonly currentDC = computed(() => this.currentRack()?.dcId ?? 'ams-01');

  constructor() {
    effect(() => {
      if (!this.currentRackId()) {
        const first = this.mutableRacks()[0];
        if (first) {
          this.router.navigate(['/racks', first.id], { replaceUrl: true });
        }
      }
    });
    effect(() => {
      const el = this.rackSheetEl()?.nativeElement as { show?: () => void; hide?: () => void };
      if (this.editRack() !== null) el?.show?.();
      else el?.hide?.();
    });
    effect(() => {
      const el = this.rackModalEl()?.nativeElement as { show?: () => void; hide?: () => void };
      if (this.deleteRack() !== null) el?.show?.();
      else el?.hide?.();
    });
    effect(() => {
      const el = this.deviceSheetEl()?.nativeElement as { show?: () => void; hide?: () => void };
      if (this.addDeviceForm() !== null) el?.show?.();
      else el?.hide?.();
    });
    effect(() => {
      const el = this.deviceModalEl()?.nativeElement as { show?: () => void; hide?: () => void };
      if (this.deleteDeviceTarget() !== null) el?.show?.();
      else el?.hide?.();
    });
    effect(() => {
      this.currentRackId();
      this.editMode.set(false);
    });
  }

  readonly filteredRacks = computed(() => {
    const dc = this.currentDC();
    const q = this.searchQuery().toLowerCase();
    return this.mutableRacks().filter(
      (r) => r.dcId === dc && (!q || r.name.toLowerCase().includes(q)),
    );
  });

  readonly currentRack = computed(() => {
    const id = this.currentRackId();
    return id ? (this.mutableRacks().find((r) => r.id === id) ?? null) : null;
  });

  readonly rackStats = computed(() => {
    const rack = this.currentRack();
    if (!rack) return { usedU: 0, freeU: 0, totalU: 42, totalPowerW: 0, deviceCount: 0 };
    const usedU = rack.devices.reduce((sum, d) => sum + d.uSize, 0);
    const totalPowerW = rack.devices.reduce((sum, d) => sum + (d.ipmi?.averageW ?? 0), 0);
    return {
      usedU,
      freeU: rack.totalU - usedU,
      totalU: rack.totalU,
      totalPowerW,
      deviceCount: rack.devices.length,
    };
  });

  readonly breadcrumbRack = computed(() => this.currentRack()?.name ?? null);

  readonly currentRackNotes = computed(() => {
    const id = this.currentRackId();
    return id ? (RACK_NOTES[id] ?? null) : null;
  });

  readonly currentRackHistory = computed(() => {
    const id = this.currentRackId();
    return id ? (RACK_HISTORY[id] ?? []) : [];
  });

  // ── CRUD actions ───────────────────────────────────────────────────────────

  openCreateRack(): void {
    this.editRack.set({ id: '', name: '', dcId: this.currentDC(), totalU: 42, devices: [] });
  }

  openEditRack(rack: Rack): void {
    this.editRack.set({ ...rack });
  }

  closeRackForm(): void {
    this.editRack.set(null);
  }

  saveRack(): void {
    const form = this.editRack();
    if (!form) return;
    const name = (this.fRackName()?.nativeElement as HTMLInputElement)?.value ?? '';
    const dcId = (this.fRackDcId()?.nativeElement as HTMLInputElement)?.value ?? '';
    const totalU =
      parseInt((this.fRackTotalU()?.nativeElement as HTMLInputElement)?.value ?? '42', 10) || 42;
    const updated: Rack = {
      id: form.id || `rack-${Date.now()}`,
      name,
      dcId,
      totalU,
      devices: form.devices ?? [],
    };
    if (form.id) {
      firstValueFrom(this.rackApi.updateRack(form.id, name, totalU))
        .then(() => {
          this.mutableRacks.update((list) => list.map((r) => (r.id === form.id ? updated : r)));
          this.editRack.set(null);
        })
        // eslint-disable-next-line no-console
        .catch((err) => console.error(connectErrorMessage(err)));
    } else {
      firstValueFrom(this.rackApi.createRack(name, totalU, ''))
        .then((res) => {
          const created = { ...updated, id: res.rackId ?? updated.id };
          this.mutableRacks.update((list) => [...list, created]);
          this.router.navigate(['/racks', created.id]);
          this.editRack.set(null);
        })
        // eslint-disable-next-line no-console
        .catch((err) => console.error(connectErrorMessage(err)));
    }
  }

  openDeleteRack(rack: Rack): void {
    this.deleteRack.set(rack);
  }

  cancelDeleteRack(): void {
    this.deleteRack.set(null);
  }

  confirmDeleteRack(): void {
    const target = this.deleteRack();
    if (!target) return;
    firstValueFrom(this.rackApi.deleteRack(target.id))
      .then(() => {
        this.mutableRacks.update((list) => list.filter((r) => r.id !== target.id));
        const remaining = this.mutableRacks().filter((r) => r.dcId === this.currentDC());
        if (remaining.length > 0) {
          this.router.navigate(['/racks', remaining[0].id]);
        } else {
          this.router.navigate(['/racks']);
        }
        this.deleteRack.set(null);
      })
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));
  }

  // ── Edit-layout mode actions ───────────────────────────────────────────────

  toggleEditMode(): void {
    this.editMode.update((v) => !v);
  }

  applyDeviceChanges(rackId: string, devices: RackDevice[]): void {
    this.mutableRacks.update((list) =>
      list.map((r) => (r.id === rackId ? { ...r, devices } : r)),
    );
  }

  openDeleteDevice(device: RackDevice): void {
    this.deleteDeviceTarget.set(device);
  }

  cancelDeleteDevice(): void {
    this.deleteDeviceTarget.set(null);
  }

  confirmDeleteDevice(): void {
    const target = this.deleteDeviceTarget();
    const rack = this.currentRack();
    if (!target || !rack) return;
    this.applyDeviceChanges(
      rack.id,
      rack.devices.filter((d) => d.id !== target.id),
    );
    this.deleteDeviceTarget.set(null);
  }

  openAddDevice(): void {
    this.addDeviceSizeInput.set(1);
    this.addDeviceNameInput.set('');
    this.addDeviceForm.set({ name: '', type: 'machine', uSize: 1, state: 'allocated' });
  }

  closeAddDevice(): void {
    this.addDeviceForm.set(null);
  }

  onAddDeviceSizeInput(value: string): void {
    this.addDeviceSizeInput.set(parseInt(value, 10) || 1);
  }

  readonly addDeviceCanFit = computed(() => {
    const rack = this.currentRack();
    if (!rack) return false;
    return findFirstFreeSlot(rack, this.addDeviceSizeInput()) !== null;
  });

  readonly canAddDevice = computed(
    () => this.addDeviceNameInput().trim().length > 0 && this.addDeviceCanFit(),
  );

  saveDevice(): void {
    const rack = this.currentRack();
    const form = this.addDeviceForm();
    if (!rack || !form) return;
    const name = (this.fDeviceName()?.nativeElement as HTMLInputElement)?.value?.trim() ?? '';
    const type =
      ((this.fDeviceType()?.nativeElement as HTMLSelectElement)?.value as DeviceType) ??
      'machine';
    const uSize =
      parseInt((this.fDeviceUSize()?.nativeElement as HTMLInputElement)?.value ?? '1', 10) || 1;
    const state =
      ((this.fDeviceState()?.nativeElement as HTMLSelectElement)?.value as DeviceState) ??
      'allocated';
    if (!name) return;
    const slot = findFirstFreeSlot(rack, uSize);
    if (slot === null) return;
    const newDevice: RackDevice = {
      id: `dev-${Date.now()}`,
      name,
      type,
      uSize,
      state,
      uStart: slot,
    };
    this.applyDeviceChanges(rack.id, [...rack.devices, newDevice]);
    this.addDeviceForm.set(null);
  }

  readonly currentRackFreeU = computed(() => this.rackStats().freeU);

  selectDC(dc: string): void {
    this.searchQuery.set('');
    this.activeModal.set(null);
    const first = RACKS.find((r) => r.dcId === dc);
    if (first) {
      this.router.navigate(['/racks', first.id]);
    } else {
      this.router.navigate(['/racks']);
    }
  }

  selectRack(id: string): void {
    this.activeModal.set(null);
    this.router.navigate(['/racks', id]);
  }

  selectDevice(id: string): void {
    this.router.navigate(['//racks/device', id]);
  }

  openModal(modal: 'notes' | 'history'): void {
    this.activeModal.set(modal);
  }

  closeModal(): void {
    this.activeModal.set(null);
  }

  readonly rackUsedU = (rack: Rack): number => rack.devices.reduce((sum, d) => sum + d.uSize, 0);

  readonly formatPowerKw = (watts: number): string => (watts / 1000).toFixed(1);

  readonly formatDaysAgo = (daysAgo: number): string => {
    if (daysAgo === 0) return 'Today';
    if (daysAgo === 1) return '1 day ago';
    if (daysAgo < 7) return `${daysAgo} days ago`;
    const weeks = Math.floor(daysAgo / 7);
    return weeks === 1 ? '1 week ago' : `${weeks} weeks ago`;
  };

  readonly historyEventIcon = (type: RackEvent['type']): string => {
    const eventMap: Record<RackEvent['type'], string> = {
      power: 'exclamation-triangle',
      hardware: 'puzzle-piece',
      config: 'gear',
      alert: 'exclamation-triangle-filled',
    };
    return eventMap[type];
  };

  readonly historyEventIconColor = (type: RackEvent['type']): string => {
    const eventMap: Record<RackEvent['type'], string> = {
      power: 'color: #f59e0b',
      hardware: 'color: #3b82f6',
      config: 'color: #6366f1',
      alert: 'color: #ef4444',
    };
    return eventMap[type];
  };

  readonly historyEventIconBg = (type: RackEvent['type']): string => {
    const eventMap: Record<RackEvent['type'], string> = {
      power: 'bg-amber-50',
      hardware: 'bg-blue-50',
      config: 'bg-indigo-50',
      alert: 'bg-red-50',
    };
    return eventMap[type];
  };
}
