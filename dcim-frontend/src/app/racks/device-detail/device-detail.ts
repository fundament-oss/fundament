import {
  ChangeDetectionStrategy,
  Component,
  computed,
  CUSTOM_ELEMENTS_SCHEMA,
  effect,
  ElementRef,
  inject,
  signal,
  viewChild,
} from '@angular/core';
import { toSignal } from '@angular/core/rxjs-interop';
import { DOCUMENT, LowerCasePipe } from '@angular/common';
import { ActivatedRoute, Router, RouterLink } from '@angular/router';
import { timestampDate } from '@bufbuild/protobuf/wkt';
import { firstValueFrom, map } from 'rxjs';
import RackDiagramComponent from '../rack-diagram/rack-diagram';
import {
  ConnectionStatus,
  ConnectionType,
  DeviceConnection,
  DeviceHistoryAction,
  DeviceHistoryEntry,
  DeviceState,
  DeviceType,
  Rack,
  RackDevice,
  RACKS,
  DEVICE_HISTORY,
  DEVICE_CONNECTIONS,
} from '../rack.model';
import { Cable, Port, PortType, PORT_TABS, PORT_TYPE_LABEL } from '../../patch-mapping/cable.model';
import { NoteComment } from '../../inventory/inventory';
import NoteApiService from '../../inventory/note-api.service';
import PatchMappingApiService from '../../patch-mapping/patch-mapping-api.service';
import PlacementApiService from '../../inventory/placement-api.service';
import CatalogApiService from '../../catalog/catalog-api.service';
import RackApiService from '../rack-api.service';
import { ASSET_CLIENT } from '../../../connect/tokens';
import connectErrorMessage from '../../../connect/error';
import { categoryToDeviceType, cablePortFromDefinition, parseRackHeight } from '../catalog-helpers';

@Component({
  selector: 'app-device-detail',
  templateUrl: './device-detail.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [RouterLink, RackDiagramComponent, LowerCasePipe],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
})
export default class DeviceDetailComponent {
  private readonly route = inject(ActivatedRoute);

  private readonly router = inject(Router);

  private readonly placementApi = inject(PlacementApiService);

  private readonly catalogApi = inject(CatalogApiService);

  private readonly patchApi = inject(PatchMappingApiService);

  private readonly noteApi = inject(NoteApiService);

  private readonly rackApi = inject(RackApiService);

  private readonly assetClient = inject(ASSET_CLIENT);

  private readonly document = inject(DOCUMENT);

  readonly device = signal<RackDevice | undefined>(undefined);

  readonly rack = signal<Rack | undefined>(undefined);

  readonly dcLabel = signal<string>('');

  constructor() {
    effect(() => {
      const id = this.deviceId();
      if (id) this.loadDevice(id);
    });
    effect(() => {
      this.deviceId(); // track device changes
      this.document.defaultView?.scrollTo(0, 0);
    });

    effect(() => {
      const show = this.showAddPortForm();
      const el = this.portNameInput();
      if (show && el) {
        setTimeout(() => (el.nativeElement as HTMLElement).focus());
      }
    });
  }

  readonly deviceId = toSignal(this.route.paramMap.pipe(map((p) => p.get('id') ?? '')), {
    initialValue: this.route.snapshot.paramMap.get('id') ?? '',
  });

  private async loadDevice(placementId: string): Promise<void> {
    try {
      const placementRes = await firstValueFrom(this.placementApi.getPlacement(placementId));
      const placement = placementRes.placement;
      if (!placement || placement.location.case !== 'rack') {
        this.device.set(undefined);
        this.rack.set(undefined);
        this.dcLabel.set('');
        return;
      }
      const rackId = placement.location.value.rackId;
      const [assetRes, rackRes, placementsRes, catalogRes, allAssetsRes] = await Promise.all([
        firstValueFrom(this.assetClient.getAsset({ id: placement.assetId })),
        firstValueFrom(this.rackApi.getRack(rackId)),
        firstValueFrom(this.placementApi.listPlacementsByRack(rackId)),
        firstValueFrom(this.catalogApi.listCatalog()),
        firstValueFrom(this.assetClient.listAssets({})),
      ]);
      const catalogById = new Map(
        catalogRes.entries
          .filter((s) => s.entry)
          .map((s) => {
            const entry = CatalogApiService.mapCatalogEntry(s.entry!);
            return [entry.id, entry] as const;
          }),
      );
      const asset = assetRes.asset;
      const rackProto = rackRes.rack;
      if (!asset || !rackProto) {
        this.device.set(undefined);
        this.rack.set(undefined);
        return;
      }
      const catalog = catalogById.get(asset.deviceCatalogId);
      const warrantyExpiry = asset.warrantyExpiry
        ? timestampDate(asset.warrantyExpiry).toISOString().slice(0, 10)
        : undefined;
      this.device.set({
        id: placement.id,
        name: asset.assetTag || asset.id,
        type: categoryToDeviceType(catalog?.category),
        uSize: parseRackHeight(catalog?.specs),
        uStart: placement.location.value.rackUnitStart,
        state: 'allocated',
        model: catalog?.model,
        assetTag: asset.assetTag,
        warrantyExpiry,
      });
      this.notesDescription.set(asset.notes);
      const assetById = new Map(allAssetsRes.assets.map((a) => [a.id, a]));
      const devices: RackDevice[] = placementsRes.placements.flatMap((p): RackDevice[] => {
        if (p.location.case !== 'rack') return [];
        const a = assetById.get(p.assetId);
        const cat = a ? catalogById.get(a.deviceCatalogId) : undefined;
        return [
          {
            id: p.id,
            name: a?.assetTag || p.assetId,
            type: categoryToDeviceType(cat?.category),
            uSize: parseRackHeight(cat?.specs),
            uStart: p.location.value.rackUnitStart,
            state: 'allocated',
          },
        ];
      });
      this.rack.set({
        id: rackProto.id,
        name: rackProto.name,
        dcId: '',
        totalU: rackProto.totalUnits,
        devices,
      });
      this.dcLabel.set('');

      // Port definitions for every device in the rack — drives this device's
      // port list and resolves cable peer port names.
      const catalogIds = [
        ...new Set(
          placementsRes.placements
            .map((p) => assetById.get(p.assetId)?.deviceCatalogId)
            .filter((id): id is string => !!id),
        ),
      ];
      const portDefArrays = await Promise.all(
        catalogIds.map((id) => firstValueFrom(this.catalogApi.listPortDefinitions(id))),
      );
      const portDefsByCatalog = new Map(
        catalogIds.map((id, i) => [id, portDefArrays[i].portDefinitions]),
      );

      this.realPorts.set(
        (portDefsByCatalog.get(asset.deviceCatalogId) ?? [])
          .map((pd) => cablePortFromDefinition(pd, placement.id))
          .filter((p): p is Port => p !== null),
      );

      // Resolve connection peer names: placement id -> name, port def id -> port.
      const portById = new Map<string, Port>();
      placementsRes.placements.forEach((p) => {
        if (p.location.case !== 'rack') return;
        const catId = assetById.get(p.assetId)?.deviceCatalogId;
        (catId ? (portDefsByCatalog.get(catId) ?? []) : []).forEach((pd) => {
          const port = cablePortFromDefinition(pd, p.id);
          if (port) portById.set(port.id, port);
        });
      });
      const deviceNameById = new Map(devices.map((d) => [d.id, d.name]));

      const [connsRes, notesRes] = await Promise.all([
        firstValueFrom(this.patchApi.listConnectionsByPlacement(placement.id)),
        firstValueFrom(this.noteApi.listNotesForPlacement(placement.id)),
      ]);
      this.cables.set(
        connsRes.connections.map((c) =>
          PatchMappingApiService.mapConnection(c, '', { deviceNameById, portById }),
        ),
      );
      this.notes.set(notesRes.notes.map(NoteApiService.mapNote));
    } catch (err) {
      // eslint-disable-next-line no-console
      console.error(connectErrorMessage(err));
    }
  }

  /** Free-text description shown above the comment thread (the asset's notes). */
  readonly notesDescription = signal('');

  /** Comment thread for this device (placement), loaded from the note API. */
  readonly notes = signal<NoteComment[]>([]);

  // ── Port management ────────────────────────────────────────────────────────
  readonly activePortTab = signal<PortType>('network-interface');

  readonly showAddPortForm = signal(false);

  private readonly portNameInput = viewChild<ElementRef>('portNameInput');

  readonly newPortName = signal('');

  private readonly extraPorts = signal<Record<string, Port[]>>({});

  /** Ports of the current device, derived from its catalog entry's port definitions. */
  private readonly realPorts = signal<Port[]>([]);

  /** Physical connections touching the current device. */
  private readonly cables = signal<Cable[]>([]);

  readonly PORT_TABS = PORT_TABS;

  readonly PORT_TYPE_LABEL = PORT_TYPE_LABEL;

  readonly devicePorts = computed<Port[]>(() => {
    const devId = this.deviceId();
    const tab = this.activePortTab();
    const base = this.realPorts();
    const extra = this.extraPorts()[devId] ?? [];
    return [...base, ...extra].filter((p) => p.type === tab);
  });

  readonly portCableMap = computed<Map<string, Cable>>(() => {
    const devId = this.deviceId();
    const cableMap = new Map<string, Cable>();
    this.cables().forEach((cable) => {
      if (cable.aSide.deviceId === devId) cableMap.set(cable.aSide.portId, cable);
      if (cable.bSide.deviceId === devId) cableMap.set(cable.bSide.portId, cable);
    });
    return cableMap;
  });

  addPort(): void {
    const name = this.newPortName().trim();
    if (!name) return;
    const devId = this.deviceId();
    const port: Port = {
      id: `p-${devId}-${Date.now()}`,
      deviceId: devId,
      name,
      type: this.activePortTab(),
    };
    this.extraPorts.update((prev) => ({
      ...prev,
      [devId]: [...(prev[devId] ?? []), port],
    }));
    this.newPortName.set('');
    this.showAddPortForm.set(false);
  }

  disconnectCable(portId: string): void {
    const cable = this.portCableMap().get(portId);
    if (!cable) return;
    firstValueFrom(this.patchApi.deletePhysicalConnection(cable.id))
      .then(() => this.loadDevice(this.deviceId()))
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));
  }

  openConnectForm(port: Port): void {
    this.router.navigate(['/patch-mapping'], {
      queryParams: { aDeviceId: port.deviceId, aPortId: port.id },
    });
  }

  readonly deviceHistory = computed<DeviceHistoryEntry[]>(
    () => DEVICE_HISTORY[this.deviceId()] ?? [],
  );

  readonly deviceConnections = computed<DeviceConnection[]>(
    () => DEVICE_CONNECTIONS[this.deviceId()] ?? [],
  );

  readonly allDevices = computed<Map<string, RackDevice>>(() => {
    const devMap = new Map<string, RackDevice>();
    RACKS.forEach((rack) => {
      rack.devices.forEach((d) => devMap.set(d.id, d));
    });
    return devMap;
  });

  remoteDevice(id: string): RackDevice | undefined {
    return this.allDevices().get(id);
  }

  readonly connectionTypeIcon = (type: ConnectionType): string => {
    const icons: Record<ConnectionType, string> = {
      network: 'ti-network',
      power: 'ti-bolt',
      management: 'ti-terminal-2',
      storage: 'ti-database',
    };
    return icons[type];
  };

  readonly connectionTypeColor = (type: ConnectionType): string => {
    const colors: Record<ConnectionType, string> = {
      network: 'text-indigo-500 bg-indigo-50',
      power: 'text-amber-500 bg-amber-50',
      management: 'text-teal-500 bg-teal-50',
      storage: 'text-blue-500 bg-blue-50',
    };
    return colors[type];
  };

  readonly connectionStatusDot = (status: ConnectionStatus): string => {
    if (status === 'up') return 'bg-emerald-500';
    if (status === 'down') return 'bg-red-500';
    return 'bg-gray-400';
  };

  readonly connectionStatusLabel = (status: ConnectionStatus): string => {
    if (status === 'up') return 'Up';
    if (status === 'down') return 'Down';
    return 'Unknown';
  };

  readonly remoteDeviceRackName = (deviceId: string): string => {
    const rack = RACKS.find((r) => r.devices.some((d) => d.id === deviceId));
    return rack ? rack.name : '';
  };

  navigateToDevice(id: string): void {
    this.router.navigate(['//racks/device', id]);
  }

  readonly uRange = (device: RackDevice): string => {
    const end = device.uStart + device.uSize - 1;
    return device.uSize === 1 ? `U${device.uStart}` : `U${device.uStart} – U${end}`;
  };

  readonly stateBadgeClass = (state: DeviceState): string => {
    const stateMap: Record<DeviceState, string> = {
      allocated: 'bg-indigo-100 text-indigo-700',
      free: 'bg-gray-100 text-gray-600',
      offline: 'bg-red-100 text-red-700',
      locked: 'bg-violet-100 text-violet-700',
      reserved: 'bg-sky-100 text-sky-700',
    };
    return stateMap[state];
  };

  readonly powerBadgeClass = (powerstate: 'ON' | 'OFF'): string =>
    powerstate === 'ON' ? 'bg-teal-100 text-teal-700' : 'bg-red-100 text-red-600';

  readonly livelinessClass = (liveliness: 'Alive' | 'Dead' | 'Unknown' | undefined): string => {
    if (liveliness === 'Alive') return 'bg-emerald-500';
    if (liveliness === 'Dead') return 'bg-red-500';
    return 'bg-gray-400';
  };

  readonly deviceTypeLabel = (type: DeviceType): string => {
    const typeMap: Record<DeviceType, string> = {
      machine: 'Server',
      switch: 'Network Switch',
      patch: 'Patch Panel',
      pdu: 'PDU',
    };
    return typeMap[type];
  };

  readonly chassisFrontClass = (device: RackDevice): string => {
    const stateMap: Record<DeviceState, string> = {
      allocated: 'bg-indigo-500 border-indigo-700',
      free: 'bg-gray-400 border-gray-600',
      offline: 'bg-red-500 border-red-700',
      locked: 'bg-violet-500 border-violet-700',
      reserved: 'bg-sky-500 border-sky-700',
    };
    return stateMap[device.state];
  };

  readonly formatMemory = (mb: number): string => (mb >= 1024 ? `${mb / 1024} TB` : `${mb} GB`);

  readonly chassisHeight = (device: RackDevice): number => Math.max(56, device.uSize * 44);

  readonly driveBays = (device: RackDevice): readonly number[] =>
    Array.from({ length: Math.min(device.hardware?.disks ?? 2, 8) }, (_, i) => i);

  readonly nicPorts = (device: RackDevice): readonly number[] =>
    Array.from({ length: Math.min(device.hardware?.nics ?? 1, 6) }, (_, i) => i);

  readonly formatDaysAgo = (daysAgo: number): string => {
    if (daysAgo === 0) return 'Today';
    if (daysAgo === 1) return 'Yesterday';
    if (daysAgo < 30) return `${daysAgo} days ago`;
    const months = Math.floor(daysAgo / 30);
    return months === 1 ? '1 month ago' : `${months} months ago`;
  };

  readonly historyIcon = (action: DeviceHistoryAction): string => {
    const icons: Record<DeviceHistoryAction, string> = {
      'state-change': 'ti-refresh text-indigo-500',
      maintenance: 'ti-tool text-amber-500',
      allocation: 'ti-users text-teal-500',
      hardware: 'ti-cpu text-blue-500',
      created: 'ti-plus text-green-500',
    };
    return icons[action];
  };

  readonly historyIconBg = (action: DeviceHistoryAction): string => {
    const bg: Record<DeviceHistoryAction, string> = {
      'state-change': 'bg-indigo-50',
      maintenance: 'bg-amber-50',
      allocation: 'bg-teal-50',
      hardware: 'bg-blue-50',
      created: 'bg-green-50',
    };
    return bg[action];
  };
}
