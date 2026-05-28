import {
  ChangeDetectionStrategy,
  Component,
  computed,
  CUSTOM_ELEMENTS_SCHEMA,
  effect,
  inject,
  OnInit,
  signal,
  untracked,
  viewChild,
} from '@angular/core';
import { toSignal } from '@angular/core/rxjs-interop';
import { ActivatedRoute, Router } from '@angular/router';
import { Code, ConnectError } from '@connectrpc/connect';
import { firstValueFrom, map } from 'rxjs';
import RackApiService from './rack-api.service';
import DatacenterApiService from '../datacenters/datacenter-api.service';
import InventoryApiService from '../inventory/inventory-api.service';
import PlacementApiService from '../inventory/placement-api.service';
import connectErrorMessage from '../../connect/error';
import DcSelectorComponent from '../shared/dc-selector';
import RackDiagramComponent from './rack-diagram/rack-diagram';
import RackDiagramEditorComponent from './rack-diagram-editor/rack-diagram-editor';
import { Rack, RackDevice } from './rack.model';
import { DatacenterInfo, RackRow, Room } from '../datacenters/datacenter.model';
import { RackSlotType } from '../../generated/v1/common_pb';
import { ViolationsSchema } from '../../generated/buf/validate/validate_pb';

interface RackListItem extends Rack {
  usedU: number;
  freeU: number;
  totalPowerW: number;
  deviceCount: number;
  rowId: string;
}

interface RowOption {
  id: string;
  label: string;
}

interface AssetOption {
  id: string;
  label: string;
}

interface AddDeviceForm {
  assetId: string;
  rackUnitStart: number;
  slotType: RackSlotType;
}

interface PlacementInfo {
  placementId: string;
  assetId: string;
  assetTag: string;
  rackUnitStart: number;
  slotType: RackSlotType;
}

type InvalidFields = Record<string, string>;

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
export default class RacksComponent implements OnInit {
  private readonly rackApi = inject(RackApiService);

  private readonly dcApi = inject(DatacenterApiService);

  private readonly inventoryApi = inject(InventoryApiService);

  private readonly placementApi = inject(PlacementApiService);

  private readonly route = inject(ActivatedRoute);

  private readonly router = inject(Router);

  readonly slotTypes: { value: RackSlotType; label: string }[] = [
    { value: RackSlotType.UNIT, label: 'Unit' },
    { value: RackSlotType.POWER, label: 'Power' },
    { value: RackSlotType.ZERO_U, label: 'Zero-U' },
  ];

  readonly currentRackId = toSignal(this.route.paramMap.pipe(map((p) => p.get('rackId'))), {
    initialValue: this.route.snapshot.paramMap.get('rackId'),
  });

  viewMode = signal<'front' | 'back'>('front');

  searchQuery = signal('');

  activeModal = signal<'notes' | 'history' | null>(null);

  // ── Mutable rack list (per selected DC) ────────────────────────────────────
  readonly mutableRacks = signal<RackListItem[]>([]);

  // Placements keyed by rack id. Loaded on demand via ListPlacementsByRack.
  readonly placementsByRack = signal<Map<string, PlacementInfo[]>>(new Map());

  // ── DC list (loaded from the API) ──────────────────────────────────────────
  readonly mutableDcs = signal<DatacenterInfo[]>([]);

  readonly selectedDcId = signal('');

  // ── Row options for the create-rack form (rooms + rows in the selected DC) ─
  readonly rowOptions = signal<RowOption[]>([]);

  // ── CRUD state ─────────────────────────────────────────────────────────────
  readonly editRack = signal<(Partial<Rack> & { rowId?: string }) | null>(null);

  readonly rackErrorMessage = signal<string | null>(null);

  readonly invalidFields = signal<InvalidFields>({});

  readonly deleteRack = signal<Rack | null>(null);

  // ── Edit-layout mode ───────────────────────────────────────────────────────
  readonly editMode = signal(false);

  readonly deleteDeviceTarget = signal<RackDevice | null>(null);

  readonly addDeviceForm = signal<AddDeviceForm | null>(null);

  readonly assetOptions = signal<AssetOption[]>([]);

  readonly deviceErrorMessage = signal<string | null>(null);

  readonly invalidDeviceFields = signal<InvalidFields>({});

  private readonly rackSheetEl = viewChild<NativeElementRef>('rackSheet');

  private readonly rackModalEl = viewChild<NativeElementRef>('rackModal');

  private readonly fRackName = viewChild<NativeElementRef>('fRackName');

  private readonly fRackRowId = viewChild<NativeElementRef>('fRackRowId');

  private readonly fRackTotalU = viewChild<NativeElementRef>('fRackTotalU');

  private readonly deviceSheetEl = viewChild<NativeElementRef>('deviceSheet');

  private readonly deviceModalEl = viewChild<NativeElementRef>('deviceModal');

  private readonly fDeviceAsset = viewChild<NativeElementRef>('fDeviceAsset');

  private readonly fDeviceSlotType = viewChild<NativeElementRef>('fDeviceSlotType');

  private readonly fDeviceRackUnit = viewChild<NativeElementRef>('fDeviceRackUnit');

  readonly currentDC = computed(() => this.selectedDcId());

  constructor() {
    // When the selected DC changes, fetch its racks and row options.
    effect(() => {
      const dcId = this.selectedDcId();
      if (!dcId) return;
      this.reloadRacks(dcId);
      this.reloadRowOptions(dcId);
    });

    // When the selected rack changes, load its placements as devices.
    effect(() => {
      const rackId = this.currentRackId();
      if (rackId) this.reloadDevicesForRack(rackId);
    });

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

  ngOnInit(): void {
    firstValueFrom(this.dcApi.listSites())
      .then((res) => {
        const dcs = res.sites.map((s) => DatacenterApiService.mapSite(s));
        this.mutableDcs.set(dcs);
        if (!this.selectedDcId() && dcs.length > 0) {
          this.selectedDcId.set(dcs[0].id);
        }
      })
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));
  }

  private reloadRacks(dcId: string): void {
    firstValueFrom(this.rackApi.listRacksBySite(dcId))
      .then((res) => {
        const racks: RackListItem[] = res.racks.flatMap((summary): RackListItem[] => {
          const rack = summary.rack;
          if (!rack) return [];
          return [
            {
              id: rack.id,
              name: rack.name,
              dcId,
              rowId: rack.rowId,
              totalU: rack.totalUnits,
              devices: [] as RackDevice[],
              usedU: summary.usedUnits,
              freeU: summary.freeUnits,
              totalPowerW: summary.powerDrawW,
              deviceCount: summary.deviceCount,
            },
          ];
        });
        untracked(() => this.mutableRacks.set(racks));
      })
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));
  }

  private async reloadDevicesForRack(rackId: string): Promise<void> {
    try {
      const [placementsRes, assetsRes] = await Promise.all([
        firstValueFrom(this.placementApi.listPlacementsByRack(rackId)),
        firstValueFrom(
          this.inventoryApi.listAssets({ status: 'all', category: 'all', sortDirection: 'asc' }),
        ),
      ]);
      const assetTag = new Map<string, string>(
        assetsRes.assets.map((a) => [a.id, a.assetTag || a.id]),
      );
      const placements: PlacementInfo[] = placementsRes.placements.flatMap(
        (p): PlacementInfo[] => {
          if (p.location.case !== 'rack') return [];
          const loc = p.location.value;
          return [
            {
              placementId: p.id,
              assetId: p.assetId,
              assetTag: assetTag.get(p.assetId) ?? p.assetId,
              rackUnitStart: loc.rackUnitStart,
              slotType: loc.rackSlotType,
            },
          ];
        },
      );
      untracked(() => {
        this.placementsByRack.update((prev) => {
          const next = new Map(prev);
          next.set(rackId, placements);
          return next;
        });
      });
    } catch (err) {
      // eslint-disable-next-line no-console
      console.error(connectErrorMessage(err));
    }
  }

  private static placementsToDevices(placements: PlacementInfo[]): RackDevice[] {
    return placements.map((p) => ({
      id: p.placementId,
      name: p.assetTag,
      type: 'machine',
      uSize: 1,
      uStart: p.rackUnitStart,
      state: 'allocated',
    }));
  }

  private async reloadRowOptions(dcId: string): Promise<void> {
    try {
      const [roomsRes, rowsRes] = await Promise.all([
        firstValueFrom(this.dcApi.listRooms(dcId)),
        firstValueFrom(this.dcApi.listRackRowsBySite(dcId)),
      ]);
      const rooms = roomsRes.rooms.map((r) => DatacenterApiService.mapRoom(r));
      const rows = rowsRes.rackRows.map((r) => DatacenterApiService.mapRackRow(r));
      const roomName = new Map<string, string>(rooms.map((r: Room) => [r.id, r.name]));
      const options = rows.map((r: RackRow) => ({
        id: r.id,
        label: `${roomName.get(r.roomId) ?? '—'} · ${r.name}`,
      }));
      this.rowOptions.set(options);
    } catch (err) {
      // eslint-disable-next-line no-console
      console.error(connectErrorMessage(err));
    }
  }

  readonly filteredRacks = computed(() => {
    const q = this.searchQuery().toLowerCase();
    return this.mutableRacks().filter((r) => !q || r.name.toLowerCase().includes(q));
  });

  readonly currentRack = computed(() => {
    const id = this.currentRackId();
    if (!id) return null;
    const rack = this.mutableRacks().find((r) => r.id === id);
    if (!rack) return null;
    const placements = this.placementsByRack().get(id);
    const devices = placements ? RacksComponent.placementsToDevices(placements) : rack.devices;
    return { ...rack, devices };
  });

  readonly rackStats = computed(() => {
    const rack = this.currentRack();
    if (!rack) return { usedU: 0, freeU: 0, totalU: 42, totalPowerW: 0, deviceCount: 0 };
    return {
      usedU: rack.usedU,
      freeU: rack.freeU,
      totalU: rack.totalU,
      totalPowerW: rack.totalPowerW,
      deviceCount: rack.deviceCount,
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
    this.clearRackErrors();
    this.editRack.set({
      id: '',
      name: '',
      dcId: this.currentDC(),
      rowId: '',
      totalU: 42,
      devices: [],
    });
  }

  openEditRack(rack: RackListItem): void {
    this.clearRackErrors();
    this.editRack.set({ ...rack });
  }

  closeRackForm(): void {
    this.clearRackErrors();
    this.editRack.set(null);
  }

  saveRack(): void {
    const form = this.editRack();
    if (!form) return;
    this.clearRackErrors();
    const name = (this.fRackName()?.nativeElement as HTMLInputElement)?.value ?? '';
    const rowId = (this.fRackRowId()?.nativeElement as HTMLSelectElement)?.value ?? '';
    const totalU =
      parseInt((this.fRackTotalU()?.nativeElement as HTMLInputElement)?.value ?? '42', 10) || 42;
    if (form.id) {
      firstValueFrom(this.rackApi.updateRack(form.id, name, totalU))
        .then(() => {
          this.reloadRacks(this.selectedDcId());
          this.editRack.set(null);
        })
        .catch((err) => this.handleRackError(err));
    } else {
      firstValueFrom(this.rackApi.createRack(name, totalU, rowId))
        .then((res) => {
          this.reloadRacks(this.selectedDcId());
          if (res.rackId) {
            this.router.navigate(['/racks', res.rackId]);
          }
          this.editRack.set(null);
        })
        .catch((err) => this.handleRackError(err));
    }
  }

  isFieldInvalid(field: string): boolean {
    return field in this.invalidFields();
  }

  fieldError(field: string): string {
    return this.invalidFields()[field] ?? '';
  }

  private clearRackErrors(): void {
    this.invalidFields.set({});
    this.rackErrorMessage.set(null);
  }

  private handleRackError(err: unknown): void {
    const ce = ConnectError.from(err);

    if (ce.code === Code.InvalidArgument) {
      const fieldErrors: InvalidFields = {};
      const unmappedMessages: string[] = [];
      ce.findDetails(ViolationsSchema)
        .flatMap((violations) => violations.violations)
        .forEach((v) => {
          const field = v.field?.elements.map((e) => e.fieldName).join('.') ?? '';
          if (field) fieldErrors[field] = v.message;
          else unmappedMessages.push(v.message);
        });
      if (Object.keys(fieldErrors).length > 0) {
        this.invalidFields.set(fieldErrors);
        if (unmappedMessages.length > 0) {
          this.rackErrorMessage.set(unmappedMessages.join('\n'));
        }
        return;
      }
    }

    this.rackErrorMessage.set(connectErrorMessage(err));
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
        this.router.navigate(['/racks']);
        this.reloadRacks(this.selectedDcId());
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
    const placements = this.placementsByRack().get(rackId) ?? [];
    const byPid = new Map(placements.map((p) => [p.placementId, p]));
    const moves = devices.flatMap((d) => {
      const prev = byPid.get(d.id);
      if (!prev || prev.rackUnitStart === d.uStart) return [];
      return [{ placementId: d.id, newUnit: d.uStart, slotType: prev.slotType }];
    });
    if (moves.length === 0) return;
    const movedUnit = new Map(moves.map((m) => [m.placementId, m.newUnit]));
    this.placementsByRack.update((prev) => {
      const next = new Map(prev);
      next.set(
        rackId,
        placements.map((p) =>
          movedUnit.has(p.placementId)
            ? { ...p, rackUnitStart: movedUnit.get(p.placementId)! }
            : p,
        ),
      );
      return next;
    });
    Promise.all(
      moves.map((m) =>
        firstValueFrom(this.placementApi.updatePlacement(m.placementId, rackId, m.newUnit, m.slotType)),
      ),
    )
      .then(() => {
        this.reloadDevicesForRack(rackId);
        this.reloadRacks(this.selectedDcId());
      })
      .catch((err) => {
        // On failure, refetch to revert the optimistic update.
        // eslint-disable-next-line no-console
        console.error(connectErrorMessage(err));
        this.reloadDevicesForRack(rackId);
      });
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
    firstValueFrom(this.placementApi.deletePlacement(target.id))
      .then(() => {
        this.reloadDevicesForRack(rack.id);
        this.reloadRacks(this.selectedDcId());
        this.deleteDeviceTarget.set(null);
      })
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));
  }

  openAddDevice(): void {
    const rack = this.currentRack();
    if (!rack) return;
    this.clearDeviceErrors();
    const firstFree = findFirstFreeSlot(rack, 1);
    this.addDeviceForm.set({
      assetId: '',
      rackUnitStart: firstFree ?? rack.totalU,
      slotType: RackSlotType.UNIT,
    });
    firstValueFrom(
      this.inventoryApi.listAssets({ status: 'all', category: 'all', sortDirection: 'asc' }),
    )
      .then((res) => {
        this.assetOptions.set(
          res.assets.map((a) => ({
            id: a.id,
            label: a.assetTag || a.id,
          })),
        );
      })
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));
  }

  closeAddDevice(): void {
    this.clearDeviceErrors();
    this.addDeviceForm.set(null);
  }

  saveDevice(): void {
    const rack = this.currentRack();
    const form = this.addDeviceForm();
    if (!rack || !form) return;
    this.clearDeviceErrors();
    const assetId = (this.fDeviceAsset()?.nativeElement as HTMLSelectElement)?.value ?? '';
    const slotType =
      (Number((this.fDeviceSlotType()?.nativeElement as HTMLSelectElement)?.value) as RackSlotType) ||
      RackSlotType.UNIT;
    const rackUnitStart =
      parseInt((this.fDeviceRackUnit()?.nativeElement as HTMLInputElement)?.value ?? '0', 10) || 0;
    firstValueFrom(this.placementApi.createPlacement(assetId, rack.id, rackUnitStart, slotType))
      .then(() => {
        this.addDeviceForm.set(null);
        this.reloadDevicesForRack(rack.id);
        this.reloadRacks(this.selectedDcId());
      })
      .catch((err) => this.handleDeviceError(err));
  }

  isDeviceFieldInvalid(field: string): boolean {
    return field in this.invalidDeviceFields();
  }

  deviceFieldError(field: string): string {
    return this.invalidDeviceFields()[field] ?? '';
  }

  private clearDeviceErrors(): void {
    this.invalidDeviceFields.set({});
    this.deviceErrorMessage.set(null);
  }

  private handleDeviceError(err: unknown): void {
    const ce = ConnectError.from(err);

    if (ce.code === Code.InvalidArgument) {
      const fieldErrors: InvalidFields = {};
      const unmappedMessages: string[] = [];
      ce.findDetails(ViolationsSchema)
        .flatMap((violations) => violations.violations)
        .forEach((v) => {
          const field = v.field?.elements.map((e) => e.fieldName).join('.') ?? '';
          if (field) fieldErrors[field] = v.message;
          else unmappedMessages.push(v.message);
        });
      if (Object.keys(fieldErrors).length > 0) {
        this.invalidDeviceFields.set(fieldErrors);
        if (unmappedMessages.length > 0) {
          this.deviceErrorMessage.set(unmappedMessages.join('\n'));
        }
        return;
      }
    }

    this.deviceErrorMessage.set(connectErrorMessage(err));
  }

  readonly currentRackFreeU = computed(() => this.rackStats().freeU);

  selectDC(dc: string): void {
    this.searchQuery.set('');
    this.activeModal.set(null);
    this.selectedDcId.set(dc);
    this.router.navigate(['/racks']);
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

  readonly rackUsedU = (rack: RackListItem): number => rack.usedU;

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
