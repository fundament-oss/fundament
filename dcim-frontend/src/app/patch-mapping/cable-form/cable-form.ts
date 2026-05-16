import {
  ChangeDetectionStrategy,
  Component,
  computed,
  CUSTOM_ELEMENTS_SCHEMA,
  effect,
  input,
  output,
  signal,
} from '@angular/core';
import { RACKS } from '../../racks/rack.model';
import {
  Cable,
  CableColor,
  CABLE_COLOR_HEX,
  CableStatus,
  CableType,
  CABLE_TYPE_LABEL,
  DEVICE_PORTS,
  Port,
  PortType,
  PORT_TYPE_LABEL,
} from '../cable.model';
import DevicePortsComponent from '../device-ports/device-ports';

interface DeviceOption {
  id: string;
  name: string;
}

@Component({
  selector: 'app-cable-form',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [DevicePortsComponent],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  templateUrl: './cable-form.html',
})
export default class CableFormComponent {
  readonly cable = input<Partial<Cable> | null>(null);

  readonly dcId = input.required<string>();

  readonly allCables = input<Cable[]>([]);

  readonly save = output<Cable>();

  readonly cancelForm = output<void>();

  readonly cableDelete = output<Cable>();

  readonly portsUpdated = output<{ deviceId: string; ports: Port[] }>();

  // ── A Side ─────────────────────────────────────────────────────────────────
  readonly aPortType = signal<PortType | ''>('');

  readonly aDeviceId = signal('');

  readonly aPortId = signal('');

  // ── B Side ─────────────────────────────────────────────────────────────────
  readonly bPortType = signal<PortType | ''>('');

  readonly bDeviceId = signal('');

  readonly bPortId = signal('');

  // ── Cable fields ───────────────────────────────────────────────────────────
  readonly cableType = signal<CableType | ''>('');

  readonly cableStatus = signal<CableStatus>('connected');

  readonly cableLabel = signal('');

  readonly cableColor = signal<CableColor | undefined>(undefined);

  readonly cableDescription = signal('');

  readonly cableLength = signal<number | undefined>(undefined);

  // ── Port management ────────────────────────────────────────────────────────
  readonly portManagementDevice = signal<{ id: string; name: string } | null>(null);

  readonly localDevicePorts = signal<Record<string, Port[]>>({ ...DEVICE_PORTS });

  // ── Derived: devices in this DC ───────────────────────────────────────────
  readonly dcDevices = computed<DeviceOption[]>(() => {
    const dcId = this.dcId();
    const result = RACKS.filter((rack) => rack.dcId === dcId).flatMap((rack) =>
      rack.devices.map((dev) => ({ id: dev.id, name: dev.name })),
    );
    return result.sort((a, b) => a.name.localeCompare(b.name));
  });

  // ── Derived: port IDs already occupied by other cables ────────────────────
  readonly usedPortIds = computed<Set<string>>(() => {
    const editingId = this.cable()?.id;
    const set = new Set<string>();
    this.allCables().forEach((c) => {
      if (c.id === editingId) return;
      set.add(c.aSide.portId);
      set.add(c.bSide.portId);
    });
    return set;
  });

  // ── Derived: filtered port lists ──────────────────────────────────────────
  readonly aFilteredPorts = computed<Port[]>(() => {
    const devId = this.aDeviceId();
    if (!devId) return [];
    const ports = this.localDevicePorts()[devId] ?? [];
    const type = this.aPortType();
    return type ? ports.filter((p) => p.type === type) : ports;
  });

  readonly bFilteredPorts = computed<Port[]>(() => {
    const devId = this.bDeviceId();
    if (!devId) return [];
    const ports = this.localDevicePorts()[devId] ?? [];
    const type = this.bPortType();
    return type ? ports.filter((p) => p.type === type) : ports;
  });

  // ── Derived: selected port objects ────────────────────────────────────────
  readonly aSelectedPort = computed<Port | null>(
    () => this.aFilteredPorts().find((p) => p.id === this.aPortId()) ?? null,
  );

  readonly bSelectedPort = computed<Port | null>(
    () => this.bFilteredPorts().find((p) => p.id === this.bPortId()) ?? null,
  );

  // ── Derived: validation ───────────────────────────────────────────────────
  readonly isEditMode = computed(() => !!this.cable()?.id);

  readonly isSamePort = computed(() => !!this.aPortId() && this.aPortId() === this.bPortId());

  readonly incompatibleSides = computed(() => {
    const a = this.aSelectedPort();
    const b = this.bSelectedPort();
    if (!a || !b) return false;
    const powerTypes: PortType[] = ['power-port', 'power-outlet'];
    const aIsPower = powerTypes.includes(a.type);
    const bIsPower = powerTypes.includes(b.type);
    return aIsPower !== bIsPower;
  });

  readonly canSave = computed(
    () =>
      !!(
        this.aDeviceId() &&
        this.aPortId() &&
        this.bDeviceId() &&
        this.bPortId() &&
        this.cableType()
      ) &&
      !this.isSamePort() &&
      !this.incompatibleSides(),
  );

  // ── Derived: port management device ports ────────────────────────────────
  readonly portManagementPorts = computed<Port[]>(() => {
    const dev = this.portManagementDevice();
    if (!dev) return [];
    return this.localDevicePorts()[dev.id] ?? [];
  });

  constructor() {
    effect(() => {
      const c = this.cable();
      if (!c) return;
      if (c.aSide) {
        this.aPortType.set(c.aSide.portType);
        this.aDeviceId.set(c.aSide.deviceId);
        this.aPortId.set(c.aSide.portId);
      } else {
        this.aPortType.set('');
        this.aDeviceId.set('');
        this.aPortId.set('');
      }
      if (c.bSide) {
        this.bPortType.set(c.bSide.portType);
        this.bDeviceId.set(c.bSide.deviceId);
        this.bPortId.set(c.bSide.portId);
      } else {
        this.bPortType.set('');
        this.bDeviceId.set('');
        this.bPortId.set('');
      }
      this.cableType.set(c.type ?? '');
      this.cableStatus.set(c.status ?? 'connected');
      this.cableLabel.set(c.label ?? '');
      this.cableColor.set(c.color ?? undefined);
      this.cableLength.set(c.length ?? undefined);

      // Merge comments into description when loading
      const desc = c.description ?? '';
      const comments = c.comments ?? '';
      let combined: string;
      if (comments) {
        combined = desc ? `${desc}\n${comments}` : comments;
      } else {
        combined = desc;
      }
      this.cableDescription.set(combined);
    });
  }

  // ── Cascade handlers ───────────────────────────────────────────────────────

  onAPortTypeChange(value: string): void {
    this.aPortType.set(value as PortType | '');
    this.aDeviceId.set('');
    this.aPortId.set('');
  }

  onADeviceChange(value: string): void {
    this.aDeviceId.set(value);
    this.aPortId.set('');
  }

  onBPortTypeChange(value: string): void {
    this.bPortType.set(value as PortType | '');
    this.bDeviceId.set('');
    this.bPortId.set('');
  }

  onBDeviceChange(value: string): void {
    this.bDeviceId.set(value);
    this.bPortId.set('');
  }

  swapSides(): void {
    const aType = this.aPortType();
    const aDevice = this.aDeviceId();
    const aPort = this.aPortId();
    this.aPortType.set(this.bPortType());
    this.aDeviceId.set(this.bDeviceId());
    this.aPortId.set(this.bPortId());
    this.bPortType.set(aType);
    this.bDeviceId.set(aDevice);
    this.bPortId.set(aPort);
  }

  // ── Port management ────────────────────────────────────────────────────────

  openPortManagement(deviceId: string): void {
    const device = this.dcDevices().find((d) => d.id === deviceId);
    if (!device) return;
    this.portManagementDevice.set({ id: device.id, name: device.name });
  }

  closePortManagement(): void {
    this.portManagementDevice.set(null);
  }

  onPortsSaved(ports: Port[]): void {
    const dev = this.portManagementDevice();
    if (!dev) return;
    this.localDevicePorts.update((map) => ({ ...map, [dev.id]: ports }));
    // Clear selected port if it no longer exists
    if (this.aDeviceId() === dev.id && !ports.find((p) => p.id === this.aPortId())) {
      this.aPortId.set('');
    }
    if (this.bDeviceId() === dev.id && !ports.find((p) => p.id === this.bPortId())) {
      this.bPortId.set('');
    }
    this.portsUpdated.emit({ deviceId: dev.id, ports });
    this.portManagementDevice.set(null);
  }

  // ── Actions ────────────────────────────────────────────────────────────────

  onSave(): void {
    if (!this.canSave()) return;

    const aDevId = this.aDeviceId();
    const aPortId = this.aPortId();
    const bDevId = this.bDeviceId();
    const bPortId = this.bPortId();

    const ports = this.localDevicePorts();
    const aPort = (ports[aDevId] ?? []).find((p) => p.id === aPortId);
    const bPort = (ports[bDevId] ?? []).find((p) => p.id === bPortId);
    const aDevice = this.dcDevices().find((d) => d.id === aDevId);
    const bDevice = this.dcDevices().find((d) => d.id === bDevId);

    if (!aPort || !bPort || !aDevice || !bDevice) return;

    const cable: Cable = {
      id: this.cable()?.id ?? '',
      dcId: this.dcId(),
      aSide: {
        deviceId: aDevId,
        deviceName: aDevice.name,
        portId: aPortId,
        portName: aPort.name,
        portType: aPort.type,
      },
      bSide: {
        deviceId: bDevId,
        deviceName: bDevice.name,
        portId: bPortId,
        portName: bPort.name,
        portType: bPort.type,
      },
      type: this.cableType() as CableType,
      status: this.cableStatus(),
      label: this.cableLabel() || undefined,
      color: this.cableColor(),
      description: this.cableDescription() || undefined,
      length: this.cableLength(),
    };
    this.save.emit(cable);
  }

  onCancel(): void {
    this.cancelForm.emit();
  }

  onDelete(): void {
    const c = this.cable();
    if (c?.id) this.cableDelete.emit(c as Cable);
  }

  // ── Constants for template ─────────────────────────────────────────────────

  readonly PORT_TYPES: { value: PortType; label: string }[] = [
    { value: 'network-interface', label: 'Network Interface' },
    { value: 'console-port', label: 'Console Port' },
    { value: 'console-server-port', label: 'Console Server Port' },
    { value: 'power-port', label: 'Power Port' },
    { value: 'power-outlet', label: 'Power Outlet' },
  ];

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

  readonly CABLE_COLORS: CableColor[] = [
    'dark-grey',
    'light-grey',
    'red',
    'green',
    'blue',
    'yellow',
    'purple',
    'orange',
    'teal',
    'white',
  ];

  readonly CABLE_COLOR_HEX = CABLE_COLOR_HEX;

  readonly CABLE_TYPE_LABEL = CABLE_TYPE_LABEL;

  readonly PORT_TYPE_LABEL = PORT_TYPE_LABEL;
}
