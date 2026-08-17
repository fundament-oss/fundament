import {
  ChangeDetectionStrategy,
  Component,
  computed,
  CUSTOM_ELEMENTS_SCHEMA,
  effect,
  inject,
  OnInit,
  OnDestroy,
  AfterViewInit,
  TemplateRef,
  signal,
  untracked,
  viewChild,
} from '@angular/core';
import { toSignal } from '@angular/core/rxjs-interop';
import { ActivatedRoute, Router } from '@angular/router';
import { firstValueFrom, map } from 'rxjs';
import SecondaryNavService from '../shell/secondary-nav.service';
import RackApiService from './rack-api.service';
import DatacenterApiService from '../datacenters/datacenter-api.service';
import DatacenterListService from '../datacenters/datacenter-list.service';
import RackListService, { RackListItem } from './rack-list.service';
import RackNavComponent from './rack-nav';
import InventoryApiService from '../inventory/inventory-api.service';
import PlacementApiService from '../inventory/placement-api.service';
import CatalogApiService from '../catalog/catalog-api.service';
import connectErrorMessage from '../../connect/error';
import { DeviceState, DeviceType, Rack, RackDevice } from './rack.model';
import { RackRow, Room } from '../datacenters/datacenter.model';
import { RackSlotType } from '../../generated/v1/common_pb';
import parseValidationError from '../../connect/validation';
import { categoryToDeviceType, parseRackHeight } from './catalog-helpers';

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
  uSize: number;
  deviceType: ReturnType<typeof categoryToDeviceType>;
  category: string;
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

/** One row of the rack list: a device with the units it fills, or a free unit. */
interface RackRowItem {
  key: string;
  unit: number;
  label: string;
  device: RackDevice | null;
}

// ── Component ─────────────────────────────────────────────────────────────────

@Component({
  selector: 'app-racks',
  templateUrl: './racks.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [
    RackNavComponent,
  ],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
})
export default class RacksComponent implements OnInit, AfterViewInit, OnDestroy {
  private readonly secondaryNav = inject(SecondaryNavService);

  /** The rack list, handed to the shell for as long as this page is open. */
  private readonly secondaryNavTemplate = viewChild.required<TemplateRef<unknown>>('secondaryNav');

  private readonly rackApi = inject(RackApiService);

  private readonly dcApi = inject(DatacenterApiService);

  private readonly inventoryApi = inject(InventoryApiService);

  private readonly placementApi = inject(PlacementApiService);

  private readonly catalogApi = inject(CatalogApiService);

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

  activeModal = signal<'notes' | 'history' | null>(null);

  // ── Mutable rack list (per selected DC) ────────────────────────────────────
  private readonly rackList = inject(RackListService);

  /** The list this section shares with the menu and the device page. */
  readonly mutableRacks = this.rackList.racks;

  readonly racksLoaded = this.rackList.loaded;

  // Placements keyed by rack id. Loaded on demand via ListPlacementsByRack.
  readonly placementsByRack = signal<Map<string, PlacementInfo[]>>(new Map());

  // ── DC list (loaded from the API) ──────────────────────────────────────────
  private readonly dcList = inject(DatacenterListService);

  /** The list this section shares with the data center pages, so the toggle
   *  above the racks is filled the moment this page opens. */
  readonly mutableDcs = this.dcList.datacenters;

  readonly selectedDcId = this.rackList.selectedDcId;

  // ── Row options for the create-rack form (rooms + rows in the selected DC) ─
  /** The halls of the data center the form is set to. */
  readonly formRooms = signal<Room[]>([]);

  /** Every rack row in that data center, each knowing its hall. */
  readonly formRows = signal<RackRow[]>([]);

  readonly formRoomId = signal('');

  readonly formRowId = signal('');

  /** The rows of the chosen hall, which is what the second group offers. */
  readonly rowsForFormRoom = computed(() =>
    this.formRows().filter((row) => row.roomId === this.formRoomId()),
  );

  // ── CRUD state ─────────────────────────────────────────────────────────────
  readonly editRack = signal<(Partial<Rack> & { rowId?: string }) | null>(null);

  readonly rackErrorMessage = signal<string | null>(null);

  readonly invalidFields = signal<InvalidFields>({});

  readonly deleteRack = signal<Rack | null>(null);

  // ── Edit-layout mode ───────────────────────────────────────────────────────
  readonly deleteDeviceTarget = signal<RackDevice | null>(null);

  readonly addDeviceForm = signal<AddDeviceForm | null>(null);

  readonly deviceSlotType = signal<RackSlotType>(RackSlotType.UNIT);

  readonly assetOptions = signal<AssetOption[]>([]);

  readonly deviceErrorMessage = signal<string | null>(null);

  readonly invalidDeviceFields = signal<InvalidFields>({});

  private readonly rackSheetEl = viewChild<NativeElementRef>('rackSheet');

  private readonly rackModalEl = viewChild<NativeElementRef>('rackModal');

  private readonly fRackName = viewChild<NativeElementRef>('fRackName');

  private readonly fRackTotalU = viewChild<NativeElementRef>('fRackTotalU');

  private readonly deviceSheetEl = viewChild<NativeElementRef>('deviceSheet');

  private readonly deviceModalEl = viewChild<NativeElementRef>('deviceModal');

  private readonly fDeviceAsset = viewChild<NativeElementRef>('fDeviceAsset');

  private readonly fDeviceRackUnit = viewChild<NativeElementRef>('fDeviceRackUnit');

  readonly currentDC = computed(() => this.selectedDcId());

  /** What the current data center is called, for the label of the rack list. */
  readonly currentDcName = computed(
    () => this.mutableDcs().find((dc) => dc.id === this.currentDC())?.name ?? '',
  );

  /** ?new=1 means "the create form is open". The menu sets it when you press
   *  Add rack on a page that does not have this form, and it is read here
   *  rather than in ngOnInit because both addresses share one route: coming
   *  from a rack, this component is reused and never starts again. */
  private readonly createRequested = toSignal(
    this.route.queryParamMap.pipe(map((params) => params.has('new'))),
    { initialValue: this.route.snapshot.queryParamMap.has('new') },
  );

  constructor() {
    effect(() => {
      if (!this.createRequested()) return;
      untracked(() => {
        this.openCreateRack();
        this.router.navigate([], { queryParams: {}, replaceUrl: true });
      });
    });
    // A rack always stands in a data center, so the list opens in one: the
    // first, until you pick another.
    effect(() => {
      const dcs = this.mutableDcs();
      untracked(() => {
        if (!this.selectedDcId() && dcs.length > 0) this.selectedDcId.set(dcs[0].id);
      });
    });
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

    // Nothing opens on its own: /racks is the list, and which rack is open is
    // what the address says. Switching data center leaves the address pointing
    // at a rack that is not in the list any more, and that clears it.
    effect(() => {
      const racks = this.mutableRacks();
      const id = this.currentRackId();
      if (!id || racks.length === 0) return;
      if (!racks.some((rack) => rack.id === id)) {
        untracked(() => this.router.navigate(['/racks'], { replaceUrl: true }));
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
  }

  ngAfterViewInit(): void {
    this.secondaryNav.set(this.secondaryNavTemplate());
  }

  ngOnDestroy(): void {
    this.secondaryNav.clear(this.secondaryNavTemplate());
  }

  ngOnInit(): void {
    this.dcList.load();
  }

  private reloadRacks(dcId: string): void {
    this.rackList.load(dcId);
  }

  private async reloadDevicesForRack(rackId: string): Promise<void> {
    try {
      const [placementsRes, assetsRes, catalogRes] = await Promise.all([
        firstValueFrom(this.placementApi.listPlacementsByRack(rackId)),
        firstValueFrom(
          this.inventoryApi.listAssets({ status: 'all', category: 'all', sortDirection: 'asc' }),
        ),
        firstValueFrom(this.catalogApi.listCatalog()),
      ]);
      const catalogById = new Map(
        catalogRes.entries
          .filter((s) => s.entry)
          .map((s) => {
            const entry = CatalogApiService.mapCatalogEntry(s.entry!);
            return [entry.id, entry] as const;
          }),
      );
      const assetById = new Map(assetsRes.assets.map((a) => [a.id, a]));
      const placements: PlacementInfo[] = placementsRes.placements.flatMap((p): PlacementInfo[] => {
        if (p.location.case !== 'rack') return [];
        const loc = p.location.value;
        const asset = assetById.get(p.assetId);
        const catalog = asset ? catalogById.get(asset.deviceCatalogId) : undefined;
        return [
          {
            placementId: p.id,
            assetId: p.assetId,
            assetTag: asset?.assetTag || p.assetId,
            rackUnitStart: loc.rackUnitStart,
            slotType: loc.rackSlotType,
            uSize: parseRackHeight(catalog?.specs),
            deviceType: categoryToDeviceType(catalog?.category),
            category: catalog?.category ?? '',
          },
        ];
      });
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
      type: p.deviceType,
      category: p.category,
      uSize: p.uSize,
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
      this.formRooms.set(rooms);
      this.formRows.set(rows);
      // Both groups open on their first button, so a new rack always has a
      // place: nothing in this form is ever disabled waiting for a choice
      // above it.
      const room = rooms.find((r) => r.id === this.formRoomId()) ?? rooms[0];
      this.formRoomId.set(room?.id ?? '');
      const firstRow = rows.find((r) => r.roomId === room?.id);
      this.formRowId.set(firstRow?.id ?? '');
    } catch (err) {
      // eslint-disable-next-line no-console
      console.error(connectErrorMessage(err));
    }
  }

  /** Picking a hall offers the rows in it, and lands on the first. */
  onFormRoomToggle(id: string, selected: boolean): void {
    if (!selected || id === this.formRoomId()) return;
    this.formRoomId.set(id);
    this.formRowId.set(this.formRows().find((row) => row.roomId === id)?.id ?? '');
  }

  onFormRowToggle(id: string, selected: boolean): void {
    if (selected) this.formRowId.set(id);
  }


  readonly currentRack = computed(() => {
    const id = this.currentRackId();
    if (!id) return null;
    const rack = this.mutableRacks().find((r) => r.id === id);
    if (!rack) return null;
    const placements = this.placementsByRack().get(id);
    const devices = placements ? RacksComponent.placementsToDevices(placements) : rack.devices;
    return { ...rack, devices };
  });

  /**
   * The rack as a list: one row per device, however many units it takes, and
   * one row per free unit. Counted down from the top, the way you stand in
   * front of it.
   */
  readonly rackRows = computed((): RackRowItem[] => {
    const rack = this.currentRack();
    if (!rack) return [];
    const byUnit = new Map<number, RackDevice>();
    rack.devices.forEach((device) => {
      RacksComponent.span(device).forEach((u) => byUnit.set(u, device));
    });
    const rows: RackRowItem[] = [];
    for (let u = rack.totalU; u >= 1; u -= 1) {
      const device = byUnit.get(u) ?? null;
      if (!device) {
        rows.push({ key: `u${u}`, unit: u, label: String(u), device: null });
      } else if (u === device.uStart + device.uSize - 1) {
        // A device fills several units but is one row, written down at the top
        // unit of its span and skipped for the rest.
        const label = device.uSize === 1 ? String(u) : `${device.uStart}-${u}`;
        rows.push({ key: device.id, unit: device.uStart, label, device });
      }
    }
    return rows;
  });

  /** The units a device fills, from its start upwards. */
  private static span(device: RackDevice): number[] {
    return Array.from({ length: device.uSize }, (unused, i) => device.uStart + i);
  }

  /**
   * Where a device can go: every start unit whose whole span is free. A 4U
   * machine needs four units in a row, so a free unit is not a place by
   * itself. Counted down, in the order the list shows them.
   */
  moveTargets(device: RackDevice): number[] {
    const rack = this.currentRack();
    if (!rack) return [];
    const taken = new Set<number>();
    rack.devices
      .filter((other) => other.id !== device.id)
      .forEach((other) => RacksComponent.span(other).forEach((u) => taken.add(u)));
    const starts = Array.from(
      { length: rack.totalU - device.uSize + 1 },
      (unused, i) => rack.totalU - device.uSize + 1 - i,
    );
    return starts.filter(
      (start) =>
        start !== device.uStart
        && Array.from({ length: device.uSize }, (unused, i) => start + i).every(
          (u) => !taken.has(u),
        ),
    );
  }

  moveDevice(device: RackDevice, unit: number): void {
    const rack = this.currentRack();
    if (!rack) return;
    this.applyDeviceChanges(
      rack.id,
      rack.devices.map((d) => (d.id === device.id ? { ...d, uStart: unit } : d)),
    );
  }

  /**
   * What a device is. A type and a state are two different questions, and the
   * old legend answered them in one list of colours: a switch that was offline
   * came out as "Switch" and said nothing about being down. The type is a tag
   * behind the name, the state a dot on the right.
   */
  readonly deviceTypeText = (device: RackDevice): string => device.category ?? '';

  /** What the thing is, in front of its name. */
  readonly deviceTypeIcon = (device: RackDevice): string => {
    const icons: Record<DeviceType, string> = {
      machine: 'server',
      switch: 'network-switch',
      patch: 'network-patch-mapping',
      pdu: 'power-plug',
    };
    return icons[device.type];
  };

  /** How a device is doing. */
  readonly deviceStateText = (device: RackDevice): string => {
    const states: Record<DeviceState, string> = {
      allocated: 'Allocated',
      free: 'Free',
      offline: 'Offline',
      locked: 'Locked',
      reserved: 'Reserved',
    };
    return states[device.state];
  };

  /** The colours the legend used, by the name the design system gives them. */
  readonly deviceStateColor = (device: RackDevice): string => {
    const colors: Record<DeviceState, string> = {
      allocated: 'lintblauw',
      free: 'neutral',
      offline: 'rood',
      locked: 'paars',
      reserved: 'lichtblauw',
    };
    return colors[device.state];
  };

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

  /** The data center the form is making a rack in. It opens on the one you are
   *  looking at, and picking another one reloads the rows to choose from. */
  readonly formDcId = signal('');

  onFormDcToggle(id: string, selected: boolean): void {
    if (!selected || id === this.formDcId()) return;
    this.formDcId.set(id);
    this.reloadRowOptions(id);
    this.formRoomId.set('');
    this.formRowId.set('');
  }

  openCreateRack(): void {
    this.clearRackErrors();
    this.formDcId.set(this.currentDC());
    this.reloadRowOptions(this.currentDC());
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
    const rowId = this.formRowId();
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
    const { fields, message } = parseValidationError(err);
    this.invalidFields.set(fields);
    this.rackErrorMessage.set(message);
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
          movedUnit.has(p.placementId) ? { ...p, rackUnitStart: movedUnit.get(p.placementId)! } : p,
        ),
      );
      return next;
    });
    Promise.all(
      moves.map((m) =>
        firstValueFrom(
          this.placementApi.updatePlacement(m.placementId, rackId, m.newUnit, m.slotType),
        ),
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

  /** @param unit the free unit you clicked; without one, the first free unit. */
  /** The slot type as buttons: a radio group, so only the button that becomes
   *  selected has anything to say. */
  onSlotTypeToggle(value: RackSlotType, selected: boolean): void {
    if (selected) this.deviceSlotType.set(value);
  }

  openAddDevice(unit?: number): void {
    const rack = this.currentRack();
    if (!rack) return;
    this.clearDeviceErrors();
    const firstFree = findFirstFreeSlot(rack, 1);
    this.addDeviceForm.set({
      assetId: '',
      rackUnitStart: unit ?? firstFree ?? rack.totalU,
      slotType: RackSlotType.UNIT,
    });
    this.deviceSlotType.set(RackSlotType.UNIT);
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
    const slotType = this.deviceSlotType();
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
    const { fields, message } = parseValidationError(err);
    this.invalidDeviceFields.set(fields);
    this.deviceErrorMessage.set(message);
  }

  readonly currentRackFreeU = computed(() => this.rackStats().freeU);

  /** The toggle above the list: a radio group, so only the button that becomes
   *  selected has anything to say. */
  onDcToggle(id: string, selected: boolean): void {
    if (selected && id !== this.currentDC()) this.selectDC(id);
  }

  selectDC(dc: string): void {
    this.activeModal.set(null);
    this.selectedDcId.set(dc);
    // No navigation here: the rack in the address is not in this data center,
    // so the effect above steps to the first one that is.
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
      power: 'bg-amber-50 dark:bg-amber-950',
      hardware: 'bg-blue-50 dark:bg-blue-950',
      config: 'bg-indigo-50 dark:bg-indigo-950',
      alert: 'bg-red-50 dark:bg-red-950',
    };
    return eventMap[type];
  };
}
