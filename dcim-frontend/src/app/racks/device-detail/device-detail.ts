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
import { LowerCasePipe } from '@angular/common';
import { ActivatedRoute, Router, RouterLink } from '@angular/router';
import { map } from 'rxjs';
import RackDiagramComponent from '../rack-diagram/rack-diagram';
import {
  ConnectionStatus,
  ConnectionType,
  DeviceComment,
  DeviceConnection,
  DeviceHistoryAction,
  DeviceHistoryEntry,
  DeviceState,
  DeviceType,
  Rack,
  RackDevice,
  RACKS,
  PARTITIONS,
  DEVICE_NOTES,
  DEVICE_HISTORY,
  DEVICE_CONNECTIONS,
} from '../rack.model';
import {
  Cable,
  DEVICE_PORTS,
  MOCK_CABLES,
  Port,
  PortType,
  PORT_TABS,
  PORT_TYPE_LABEL,
} from '../../patch-mapping/cable.model';

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

  private readonly scrollContainer = viewChild<ElementRef<HTMLElement>>('scrollContainer');

  constructor() {
    effect(() => {
      this.deviceId(); // track device changes
      this.scrollContainer()?.nativeElement.scrollTo({ top: 0 });
    });
  }

  readonly deviceId = toSignal(this.route.paramMap.pipe(map((p) => p.get('id') ?? '')), {
    initialValue: this.route.snapshot.paramMap.get('id') ?? '',
  });

  readonly device = computed<RackDevice | undefined>(() => {
    const id = this.deviceId();
    let found: RackDevice | undefined;
    RACKS.forEach((rack) => {
      if (!found) found = rack.devices.find((d) => d.id === id);
    });
    return found;
  });

  readonly rack = computed<Rack | undefined>(() => {
    const id = this.deviceId();
    return RACKS.find((r) => r.devices.some((d) => d.id === id));
  });

  readonly dcLabel = computed(() => {
    const r = this.rack();
    return r ? (PARTITIONS.find((p) => p.id === r.dcId)?.label ?? r.dcId) : '';
  });

  readonly newNoteText = signal('');

  private readonly extraComments = signal<Record<string, DeviceComment[]>>({});

  // ── Port management ────────────────────────────────────────────────────────
  readonly activePortTab = signal<PortType>('network-interface');

  readonly showAddPortForm = signal(false);

  readonly newPortName = signal('');

  readonly newPortLabel = signal('');

  private readonly extraPorts = signal<Record<string, Port[]>>({});

  private readonly removedCableIds = signal<Set<string>>(new Set());

  readonly PORT_TABS = PORT_TABS;

  readonly PORT_TYPE_LABEL = PORT_TYPE_LABEL;

  readonly devicePorts = computed<Port[]>(() => {
    const devId = this.deviceId();
    const tab = this.activePortTab();
    const base = DEVICE_PORTS[devId] ?? [];
    const extra = this.extraPorts()[devId] ?? [];
    return [...base, ...extra].filter((p) => p.type === tab);
  });

  readonly portCableMap = computed<Map<string, Cable>>(() => {
    const devId = this.deviceId();
    const removed = this.removedCableIds();
    const cableMap = new Map<string, Cable>();
    MOCK_CABLES.filter((cable) => !removed.has(cable.id)).forEach((cable) => {
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
      label: this.newPortLabel().trim() || undefined,
      type: this.activePortTab(),
    };
    this.extraPorts.update((prev) => ({
      ...prev,
      [devId]: [...(prev[devId] ?? []), port],
    }));
    this.newPortName.set('');
    this.newPortLabel.set('');
    this.showAddPortForm.set(false);
  }

  disconnectCable(portId: string): void {
    const cable = this.portCableMap().get(portId);
    if (!cable) return;
    this.removedCableIds.update((prev) => new Set([...prev, cable.id]));
  }

  openConnectForm(port: Port): void {
    this.router.navigate(['/patch-mapping'], {
      queryParams: { aDeviceId: port.deviceId, aPortId: port.id },
    });
  }

  readonly deviceNotesDescription = computed<string>(
    () => DEVICE_NOTES[this.deviceId()]?.description ?? '',
  );

  readonly deviceComments = computed<DeviceComment[]>(() => {
    const base = DEVICE_NOTES[this.deviceId()]?.comments ?? [];
    const extra = this.extraComments()[this.deviceId()] ?? [];
    return [...base, ...extra];
  });

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

  addNote(): void {
    const text = this.newNoteText().trim();
    if (!text) return;
    const id = this.deviceId();
    const comment: DeviceComment = { author: 'You', initials: 'Y', daysAgo: 0, content: text };
    this.extraComments.update((prev) => ({
      ...prev,
      [id]: [...(prev[id] ?? []), comment],
    }));
    this.newNoteText.set('');
  }

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
