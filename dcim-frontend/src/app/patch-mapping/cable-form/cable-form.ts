import {
  afterNextRender,
  ChangeDetectionStrategy,
  Component,
  computed,
  CUSTOM_ELEMENTS_SCHEMA,
  effect,
  ElementRef,
  inject,
  Injector,
  input,
  output,
  signal,
} from '@angular/core';
import {
  Cable,
  CableColor,
  CABLE_COLOR_HEX,
  CableStatus,
  CableType,
  CABLE_TYPE_LABEL,
  Port,
  portsAreCompatible,
  PortType,
  PORT_TYPE_LABEL,
} from '../cable.model';
import DropdownSyncDirective from '../../shared/dropdown-sync.directive';

interface DeviceOption {
  id: string;
  name: string;
}

@Component({
  selector: 'app-cable-form',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [DropdownSyncDirective],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  templateUrl: './cable-form.html',
})
export default class CableFormComponent {
  private readonly elRef = inject(ElementRef);

  private readonly injector = inject(Injector);

  readonly cable = input<Partial<Cable> | null>(null);

  readonly dcId = input.required<string>();

  readonly allCables = input<Cable[]>([]);

  readonly externalDevicePorts = input<Record<string, Port[]>>({});

  /** Selectable devices (placements) in the active datacenter. */
  readonly devices = input<DeviceOption[]>([]);

  /** Server-side validation/error message from the last save attempt. */
  readonly serverError = input<string | null>(null);

  readonly save = output<Cable>();

  readonly cancelForm = output<void>();

  readonly cableDelete = output<Cable>();

  // ── A Side ─────────────────────────────────────────────────────────────────
  readonly aPortType = signal<PortType | ''>('');

  readonly aDeviceId = signal('');

  readonly aPortId = signal('');

  // ── B Side ─────────────────────────────────────────────────────────────────
  readonly bPortType = signal<PortType | ''>('');

  readonly bDeviceId = signal('');

  readonly bPortId = signal('');

  // ── Cable fields ───────────────────────────────────────────────────────────
  readonly cableType = signal<CableType | ''>('cat5e');

  readonly cableStatus = signal<CableStatus | ''>('connected');

  readonly cableLabel = signal('');

  readonly cableColor = signal<CableColor | undefined>(undefined);

  readonly cableDescription = signal('');

  readonly cableComments = signal('');

  readonly cableLength = signal<number | undefined>(undefined);

  readonly localDevicePorts = signal<Record<string, Port[]>>({});

  // ── Derived: devices in this DC ───────────────────────────────────────────
  readonly dcDevices = computed<DeviceOption[]>(() =>
    [...this.devices()].sort((a, b) => a.name.localeCompare(b.name)),
  );

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

  // ── Derived: available port types per device ──────────────────────────────
  readonly aAvailablePortTypes = computed<Set<PortType>>(() => {
    const devId = this.aDeviceId();
    if (!devId) return new Set();
    const ports = this.localDevicePorts()[devId] ?? [];
    return new Set(ports.map((p) => p.type));
  });

  readonly bAvailablePortTypes = computed<Set<PortType>>(() => {
    const devId = this.bDeviceId();
    if (!devId) return new Set();
    const ports = this.localDevicePorts()[devId] ?? [];
    return new Set(ports.map((p) => p.type));
  });

  // Port types on B side compatible with whatever is selected on A side
  readonly bCompatiblePortTypes = computed<Set<PortType>>(() => {
    const aType = this.aPortType();
    if (!aType) return new Set(this.PORT_TYPES.map((pt) => pt.value));
    return new Set(
      this.PORT_TYPES.map((pt) => pt.value).filter((t) => portsAreCompatible(aType as PortType, t)),
    );
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
    return !portsAreCompatible(a.type, b.type);
  });

  readonly canSave = computed(
    () =>
      !!(this.aDeviceId() && this.aPortId() && this.bDeviceId() && this.bPortId()) &&
      !this.isSamePort() &&
      !this.incompatibleSides(),
  );

  constructor() {
    effect(() => {
      const ext = this.externalDevicePorts();
      this.localDevicePorts.update((current) => ({ ...current, ...ext }));
    });
    effect(() => {
      const c = this.cable();
      if (!c) return;
      if (c.aSide && c.bSide && !c.id) {
        afterNextRender(() => this.focusAndScrollNameField(), { injector: this.injector });
      }
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
      // Preserve an unset (NULL) type/status when editing an existing
      // connection so we don't silently rewrite it on the next save. New
      // cables still get sensible defaults.
      const isExisting = !!c.id;
      this.cableType.set(c.type ?? (isExisting ? '' : this.CABLE_TYPES[0]));
      this.cableStatus.set(c.status ?? (isExisting ? '' : 'connected'));
      this.cableLabel.set(c.label ?? '');
      this.cableColor.set(c.color ?? undefined);
      this.cableLength.set(c.length ?? undefined);
      this.cableDescription.set(c.description ?? '');
      this.cableComments.set(c.comments ?? '');
    });
  }

  // ── Cascade handlers ───────────────────────────────────────────────────────

  onADeviceChange(value: string): void {
    this.aDeviceId.set(value);
    if (this.aPortType() && !this.aAvailablePortTypes().has(this.aPortType() as PortType)) {
      this.aPortType.set('');
    }
    this.aPortId.set('');
  }

  onAPortTypeChange(value: string): void {
    this.aPortType.set(value as PortType | '');
    this.aPortId.set('');
  }

  onBDeviceChange(value: string): void {
    this.bDeviceId.set(value);
    if (this.bPortType() && !this.bAvailablePortTypes().has(this.bPortType() as PortType)) {
      this.bPortType.set('');
    }
    this.bPortId.set('');
  }

  onBPortTypeChange(value: string): void {
    this.bPortType.set(value as PortType | '');
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
      type: this.cableType() || undefined,
      status: this.cableStatus() || undefined,
      label: this.cableLabel() || undefined,
      color: this.cableColor(),
      description: this.cableDescription() || undefined,
      comments: this.cableComments() || undefined,
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

  // ── Focus helpers ──────────────────────────────────────────────────────────

  private focusAndScrollNameField(): void {
    const el: HTMLElement | null = this.elRef.nativeElement.querySelector('#cable-label');
    if (!el) return;
    const target: HTMLElement =
      (el.shadowRoot?.querySelector('input') as HTMLElement | null) ??
      (el.querySelector('input') as HTMLElement | null) ??
      el;
    target.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
    target.focus();
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
