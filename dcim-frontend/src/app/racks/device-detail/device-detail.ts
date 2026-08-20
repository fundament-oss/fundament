import {
  AfterViewInit,
  ChangeDetectionStrategy,
  Component,
  computed,
  CUSTOM_ELEMENTS_SCHEMA,
  effect,
  ElementRef,
  inject,
  OnDestroy,
  signal,
  TemplateRef,
  viewChild,
} from '@angular/core';
import { Title } from '@angular/platform-browser';
import { toSignal } from '@angular/core/rxjs-interop';
import { DOCUMENT } from '@angular/common';
import { ActivatedRoute, Router } from '@angular/router';
import { timestampDate } from '@bufbuild/protobuf/wkt';
import { firstValueFrom, map } from 'rxjs';
import { pageTitle } from '../../shell/page-title';
import RackListService from '../rack-list.service';
import RackNavComponent from '../rack-nav';
import SecondaryNavService from '../../shell/secondary-nav.service';
import {
  ConnectionStatus,
  ConnectionType,
  DeviceState,
  DeviceType,
  Rack,
  RackDevice,
} from '../rack.model';
import {
  Cable,
  CableStatus,
  Port,
  PortType,
  PORT_TABS,
  PORT_TYPE_LABEL,
} from '../../patch-mapping/cable.model';
import { HistoryEntry, NoteComment } from '../../inventory/inventory';
import NoteApiService from '../../inventory/note-api.service';
import InventoryApiService from '../../inventory/inventory-api.service';
import PatchMappingApiService from '../../patch-mapping/patch-mapping-api.service';
import PlacementApiService from '../../inventory/placement-api.service';
import CatalogApiService from '../../catalog/catalog-api.service';
import RackApiService from '../rack-api.service';
import { ASSET_CLIENT } from '../../../connect/tokens';
import connectErrorMessage from '../../../connect/error';
import { categoryToDeviceType, cablePortFromDefinition, parseRackHeight } from '../catalog-helpers';

/** A physical connection of this device, rendered in the Connections panel. */
interface NativeElementRef {
  nativeElement: { show?: () => void; hide?: () => void };
}

interface DeviceConnectionView {
  id: string;
  localPort: string;
  remoteDeviceId: string;
  remoteDeviceName: string;
  remoteRackName: string;
  remotePort: string;
  type: ConnectionType;
  status: ConnectionStatus;
}

@Component({
  selector: 'app-device-detail',
  imports: [RackNavComponent],
  templateUrl: './device-detail.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
})
export default class DeviceDetailComponent implements AfterViewInit, OnDestroy {
  private readonly route = inject(ActivatedRoute);

  private readonly secondaryNav = inject(SecondaryNavService);

  /** This section's menu, handed to the shell for as long as the page is open. */
  private readonly secondaryNavTemplate = viewChild.required<TemplateRef<unknown>>('secondaryNav');

  ngAfterViewInit(): void {
    this.secondaryNav.set(this.secondaryNavTemplate());
  }

  ngOnDestroy(): void {
    this.secondaryNav.clear(this.secondaryNavTemplate());
  }

  private readonly router = inject(Router);

  private readonly title = inject(Title);

  private readonly placementApi = inject(PlacementApiService);

  private readonly catalogApi = inject(CatalogApiService);

  private readonly patchApi = inject(PatchMappingApiService);

  private readonly noteApi = inject(NoteApiService);

  private readonly rackList = inject(RackListService);

  private readonly inventoryApi = inject(InventoryApiService);

  private readonly rackApi = inject(RackApiService);

  private readonly assetClient = inject(ASSET_CLIENT);

  private readonly document = inject(DOCUMENT);

  readonly device = signal<RackDevice | undefined>(undefined);

  readonly rack = signal<Rack | undefined>(undefined);

  /** False until the device request settles, so "not found" only shows once it
   *  is one. Before that the page waits with an indicator. */
  readonly deviceLoaded = signal(false);

  readonly dcLabel = signal<string>('');

  constructor() {
    // The tab says which one you have open, not just which section.
    effect(() => {
      const name = this.device()?.name;
      if (name) this.title.setTitle(pageTitle(name));
    });
    effect(() => {
      const id = this.deviceId();
      if (!id) return;
      this.deviceLoaded.set(false);
      this.loadDevice(id);
    });
    effect(() => {
      this.deviceId(); // track device changes
      this.document.defaultView?.scrollTo(0, 0);
    });

    effect(() => {
      const el = this.removeModalEl()?.nativeElement;
      if (this.confirmingRemove()) el?.show?.();
      else el?.hide?.();
    });
    effect(() => {
      const el = this.portSheetEl()?.nativeElement;
      if (this.showAddPortForm()) el?.show?.();
      else el?.hide?.();
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
      // The menu beside this page lights up the rack this device stands in;
      // the address names the placement, so it cannot work that out itself.
      this.rackList.openRackId.set(rackProto.id);
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

      const [connsRes, notesRes, eventsRes] = await Promise.all([
        firstValueFrom(this.patchApi.listConnectionsByPlacement(placement.id)),
        firstValueFrom(this.noteApi.listNotesForPlacement(placement.id)),
        firstValueFrom(this.inventoryApi.getAssetEvents(asset.id)),
      ]);
      this.cables.set(
        connsRes.connections.map((c) =>
          PatchMappingApiService.mapConnection(c, '', { deviceNameById, portById }),
        ),
      );
      this.notes.set(notesRes.notes.map(NoteApiService.mapNote));
      // Newest first, the same as an asset's history: what happened last is
      // what you came to read.
      this.deviceHistory.set(
        eventsRes.events
          .map(InventoryApiService.mapAssetEvent)
          .sort((a, b) => a.daysAgo - b.daysAgo),
      );
    } catch (err) {
      // eslint-disable-next-line no-console
      console.error(connectErrorMessage(err));
    } finally {
      this.deviceLoaded.set(true);
    }
  }

  /** Free-text description shown above the comment thread (the asset's notes). */
  readonly notesDescription = signal('');

  /** Comment thread for this device (placement), loaded from the note API. */
  readonly notes = signal<NoteComment[]>([]);

  private readonly noteInput = viewChild<ElementRef>('noteInput');

  // ── Port management ────────────────────────────────────────────────────────
  /** The kind the Add port form is set to. */
  readonly newPortType = signal<PortType>('network-interface');

  readonly showAddPortForm = signal(false);

  private readonly portNameInput = viewChild<ElementRef>('portNameInput');

  private readonly portSheetEl = viewChild<NativeElementRef>('portSheet');

  readonly newPortName = signal('');

  private readonly extraPorts = signal<Record<string, Port[]>>({});

  /** Ports of the current device, derived from its catalog entry's port definitions. */
  private readonly realPorts = signal<Port[]>([]);

  /** Physical connections touching the current device. */
  private readonly cables = signal<Cable[]>([]);

  readonly PORT_TABS = PORT_TABS;

  readonly PORT_TYPE_LABEL = PORT_TYPE_LABEL;

  /**
   * Every port of this device, in one list. A tab bar over the five kinds meant
   * four empty rooms behind four clicks on a machine with two network
   * interfaces, and a count in the heading that reported the open tab rather
   * than the device. The kind rides along as a tag, as on a product.
   */
  readonly devicePorts = computed<Port[]>(() => {
    const devId = this.deviceId();
    const ports = [...this.realPorts(), ...(this.extraPorts()[devId] ?? [])];
    return PORT_TABS.flatMap((type) => ports.filter((port) => port.type === type));
  });

  /** What the row calls its kind, behind the name, the way a product's ports
   *  wear theirs. */
  readonly portTypeLabel = (type: PortType): string => PORT_TYPE_LABEL[type];

  readonly portCableMap = computed<Map<string, Cable>>(() => {
    const devId = this.deviceId();
    const cableMap = new Map<string, Cable>();
    this.cables().forEach((cable) => {
      if (cable.aSide.deviceId === devId) cableMap.set(cable.aSide.portId, cable);
      if (cable.bSide.deviceId === devId) cableMap.set(cable.bSide.portId, cable);
    });
    return cableMap;
  });

  /** The kind of port you are adding: a radio group, so only the button that
   *  becomes selected has anything to say. */
  onPortTypeToggle(type: PortType, selected: boolean): void {
    if (selected) this.newPortType.set(type);
  }

  addPort(): void {
    const name = this.newPortName().trim();
    if (!name) return;
    const devId = this.deviceId();
    const port: Port = {
      id: `p-${devId}-${Date.now()}`,
      deviceId: devId,
      name,
      type: this.newPortType(),
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

  /** Audit timeline for this device's asset, loaded from the asset-events API. */
  readonly deviceHistory = signal<HistoryEntry[]>([]);

  /** This device's physical connections, derived from the loaded cables. */
  readonly deviceConnections = computed<DeviceConnectionView[]>(() => {
    const devId = this.deviceId();
    const rackName = this.rack()?.name ?? '';
    return this.cables().flatMap((cable): DeviceConnectionView[] => {
      const localIsA = cable.aSide.deviceId === devId;
      const localIsB = cable.bSide.deviceId === devId;
      if (!localIsA && !localIsB) return [];
      const local = localIsA ? cable.aSide : cable.bSide;
      const remote = localIsA ? cable.bSide : cable.aSide;
      return [
        {
          id: cable.id,
          localPort: local.portName,
          remoteDeviceId: remote.deviceId,
          remoteDeviceName: remote.deviceName,
          // Peers resolve to a name only when they share this rack; otherwise
          // mapConnection falls back to the id and the rack is unknown.
          remoteRackName: remote.deviceName === remote.deviceId ? '' : rackName,
          remotePort: remote.portName,
          type: DeviceDetailComponent.connectionTypeFromPort(local.portType),
          status: DeviceDetailComponent.connectionStatusFromCable(cable.status),
        },
      ];
    });
  });

  private static connectionTypeFromPort(portType: PortType): ConnectionType {
    switch (portType) {
      case 'network-interface':
        return 'network';
      case 'power-port':
      case 'power-outlet':
        return 'power';
      case 'console-port':
      case 'console-server-port':
        return 'management';
      default:
        // Runs inside a computed during change detection — degrade rather than
        // crash the connections panel on an unexpected port type.
        return 'network';
    }
  }

  private static connectionStatusFromCable(status: CableStatus | undefined): ConnectionStatus {
    switch (status) {
      case 'connected':
        return 'up';
      case 'decommissioned':
        return 'down';
      case 'planned':
        return 'unknown';
      default:
        // Runs inside a computed during change detection — degrade rather than
        // crash the connections panel on an unexpected cable status.
        return 'unknown';
    }
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
      network: 'text-indigo-500 dark:text-indigo-400 bg-indigo-50 dark:bg-indigo-950',
      power: 'text-amber-500 dark:text-amber-400 bg-amber-50 dark:bg-amber-950',
      management: 'text-teal-500 dark:text-teal-400 bg-teal-50 dark:bg-teal-950',
      storage: 'text-blue-500 dark:text-blue-400 bg-blue-50 dark:bg-blue-950',
    };
    return colors[type];
  };

  readonly connectionStatusColor = (status: ConnectionStatus): string => {
    const colors: Record<ConnectionStatus, string> = {
      up: 'success',
      down: 'critical',
      unknown: 'neutral',
    };
    return colors[status];
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

  /** Up one level: the rack this device stands in, or the list of racks. */
  backToRack(): void {
    const rack = this.rack();
    this.router.navigate(rack ? ['/racks', rack.id] : ['/racks']);
  }

  navigateToDevice(id: string): void {
    this.router.navigate(['//racks/device', id]);
  }

  readonly uRange = (device: RackDevice): string => {
    const end = device.uStart + device.uSize - 1;
    return device.uSize === 1 ? `U${device.uStart}` : `U${device.uStart} – U${end}`;
  };

  /** The state as a word, not as the value the model stores it under. */
  readonly stateLabel = (state: DeviceState): string => {
    const labels: Record<DeviceState, string> = {
      allocated: 'Allocated',
      free: 'Free',
      offline: 'Offline',
      locked: 'Locked',
      reserved: 'Reserved',
    };
    return labels[state];
  };

  readonly stateTagColor = (state: DeviceState): string => {
    const stateMap: Record<DeviceState, string> = {
      allocated: 'donkerblauw',
      free: 'neutral',
      offline: 'critical',
      locked: 'violet',
      reserved: 'hemelblauw',
    };
    return stateMap[state];
  };

  readonly powerTagColor = (powerstate: 'ON' | 'OFF'): string =>
    powerstate === 'ON' ? 'success' : 'critical';

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

  addNote(): void {
    const field = this.noteInput()?.nativeElement as HTMLInputElement | undefined;
    const text = (field?.value ?? '').trim();
    if (!text) return;
    const placementId = this.deviceId();
    firstValueFrom(this.noteApi.createNoteForPlacement(placementId, text))
      .then(() => {
        if (field) field.value = '';
        return firstValueFrom(this.noteApi.listNotesForPlacement(placementId));
      })
      .then((res) => this.notes.set(res.notes.map(NoteApiService.mapNote)))
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));
  }

  /** Set while the confirmation for taking this device out of its rack is open. */
  readonly confirmingRemove = signal(false);

  private readonly removeModalEl = viewChild<NativeElementRef>('removeModal');

  removeFromRack(): void {
    this.confirmingRemove.set(true);
  }

  cancelRemove(): void {
    this.confirmingRemove.set(false);
  }

  /** Takes the device out of the rack: the placement goes, the asset stays. */
  confirmRemoveFromRack(): void {
    const rack = this.rack();
    firstValueFrom(this.placementApi.deletePlacement(this.deviceId()))
      .then(() => {
        this.confirmingRemove.set(false);
        this.router.navigate(rack ? ['/racks', rack.id] : ['/racks']);
      })
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));
  }

  /** Where a row sits in the track, so the line starts and stops in the right
   *  place. */
  historyPosition(index: number): 'first' | 'between' | 'last' | 'only' {
    const last = this.deviceHistory().length - 1;
    if (last === 0) return 'only';
    if (index === 0) return 'first';
    return index === last ? 'last' : 'between';
  }

  readonly formatDaysAgo = (daysAgo: number): string => {
    if (daysAgo === 0) return 'Today';
    if (daysAgo === 1) return 'Yesterday';
    if (daysAgo < 30) return `${daysAgo} days ago`;
    const months = Math.floor(daysAgo / 30);
    return months === 1 ? '1 month ago' : `${months} months ago`;
  };
}
