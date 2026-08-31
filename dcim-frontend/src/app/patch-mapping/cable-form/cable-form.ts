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
  CABLE_COLOR_LABEL,
  CableStatus,
  CABLE_STATUSES,
  CableType,
  CABLE_TYPE_DEFAULT_COLOR,
  CABLE_TYPE_LABEL,
  Port,
  portsAreCompatible,
  PORT_TYPE_LABEL,
} from '../cable.model';
import DevicePortsComponent from '../device-ports/device-ports';

/** A cable end: which port, on which device. */
function endKey(deviceId: string, portId: string): string {
  return `${deviceId}:${portId}`;
}

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
  private readonly elRef = inject(ElementRef);

  private readonly injector = inject(Injector);

  readonly cable = input<Partial<Cable> | null>(null);

  readonly dcId = input.required<string>();

  /** The data centers to choose from. Empty or one: the field stays hidden. */
  readonly datacenters = input<{ id: string; name: string }[]>([]);

  readonly allCables = input<Cable[]>([]);

  readonly externalDevicePorts = input<Record<string, Port[]>>({});

  /** Selectable devices (placements) in the active datacenter. */
  readonly devices = input<DeviceOption[]>([]);

  /** Server-side validation/error message from the last save attempt. */
  readonly serverError = input<string | null>(null);

  /** Asks for another data center. The sides clear here; loading what stands
   *  in the new one is the caller's job. */
  readonly dcChange = output<string>();

  readonly save = output<Cable>();

  readonly cancelForm = output<void>();

  /** Emitted when this form mutates a device's ports, so the parent can persist them. */
  readonly portsUpdated = output<{ deviceId: string; ports: Port[] }>();

  // ── A Side ─────────────────────────────────────────────────────────────────
  readonly aDeviceId = signal('');

  readonly aPortId = signal('');

  // ── B Side ─────────────────────────────────────────────────────────────────
  readonly bDeviceId = signal('');

  readonly bPortId = signal('');

  // ── Cable fields ───────────────────────────────────────────────────────────
  readonly cableType = signal<CableType | ''>('cat5e');

  readonly cableStatus = signal<CableStatus | ''>('connected');

  readonly cableLabel = signal('');

  readonly cableColor = signal<CableColor | undefined>(undefined);

  /** Whether the user explicitly chose a color; preset auto-fill stops once set. */
  private readonly colorManuallySet = signal(false);

  readonly cableDescription = signal('');

  readonly cableComments = signal('');

  readonly cableLength = signal<number | undefined>(undefined);

  readonly localDevicePorts = signal<Record<string, Port[]>>({});

  // ── Port management ────────────────────────────────────────────────────────
  readonly portManagementDevice = signal<{ id: string; name: string } | null>(null);

  // ── Derived: devices in this DC ───────────────────────────────────────────
  /**
   * The name behind an id, for the field that shows it.
   *
   * The combo box works out its own label from the item you pick, but a value
   * set from outside — the device view fills the A side — arrives before there
   * is a pick, and the field would sit there filled in and looking empty.
   */
  deviceName(id: string): string {
    return this.dcDevices().find((device) => device.id === id)?.name ?? '';
  }

  readonly dcDevices = computed<DeviceOption[]>(() =>
    [...this.devices()].sort((a, b) => a.name.localeCompare(b.name)),
  );

  /**
   * The ends other cables already occupy.
   *
   * An end is a port *on a device*, never a port on its own: a port id comes
   * from the catalog, so every device of the same model carries the same ones.
   * Keyed on the port alone, NIC 1 of one server made NIC 1 of every other
   * server of that model read as taken.
   */
  readonly usedPortEnds = computed<Set<string>>(() => {
    const editingId = this.cable()?.id;
    const set = new Set<string>();
    this.allCables().forEach((c) => {
      if (c.id === editingId) return;
      set.add(endKey(c.aSide.deviceId, c.aSide.portId));
      set.add(endKey(c.bSide.deviceId, c.bSide.portId));
    });
    return set;
  });

  portInUse(deviceId: string, port: Port): boolean {
    return this.usedPortEnds().has(endKey(deviceId, port.id));
  }

  // ── Derived: the ports of each chosen device ──────────────────────────────
  readonly aPorts = computed<Port[]>(() => this.localDevicePorts()[this.aDeviceId()] ?? []);

  readonly bPorts = computed<Port[]>(() => this.localDevicePorts()[this.bDeviceId()] ?? []);

  /** A port on the B side that cannot pair with the one chosen on the A side
   *  stays in the list and cannot be picked, the same as one already in use. */
  fitsOtherSide(port: Port): boolean {
    const a = this.aSelectedPort();
    return !a || portsAreCompatible(a.type, port.type);
  }

  // ── Derived: selected port objects ────────────────────────────────────────
  readonly aSelectedPort = computed<Port | null>(
    () => this.aPorts().find((p) => p.id === this.aPortId()) ?? null,
  );

  readonly bSelectedPort = computed<Port | null>(
    () => this.bPorts().find((p) => p.id === this.bPortId()) ?? null,
  );

  // ── Derived: validation ───────────────────────────────────────────────────
  readonly isEditMode = computed(() => !!this.cable()?.id);

  /** Both ends on one port of one device. Two devices of the same model share
   *  their port ids, so the device has to match too. */
  readonly isSamePort = computed(
    () =>
      !!this.aPortId() &&
      this.aPortId() === this.bPortId() &&
      this.aDeviceId() === this.bDeviceId(),
  );

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

  /**
   * Set by pressing Save. A field you have not filled in yet is not wrong, it
   * is unfinished, so nothing goes red until you say you are done. The two
   * clashes below do show at once: those are about a choice you just made.
   */
  readonly saveAttempted = signal(false);

  readonly aDeviceError = computed(() =>
    this.saveAttempted() && !this.aDeviceId() ? 'Choose the device this end runs to.' : '',
  );

  readonly aPortError = computed(() =>
    // Only once the field is on screen: without a device there is no port list
    // to choose from, and the device error already says so.
    this.saveAttempted() && this.aDeviceId() && !this.aPortId()
      ? 'Choose the port this end plugs into.'
      : '',
  );

  readonly bDeviceError = computed(() =>
    this.saveAttempted() && !this.bDeviceId() ? 'Choose the device this end runs to.' : '',
  );

  readonly bPortError = computed(() => {
    if (this.isSamePort()) return 'A cable cannot run from a port back to itself.';
    if (this.incompatibleSides()) return 'A power port and a data port cannot be connected.';
    if (this.saveAttempted() && this.bDeviceId() && !this.bPortId()) {
      return 'Choose the port this end plugs into.';
    }
    return '';
  });

  /** The field Save sends you to, in the order you read them. */
  private readonly firstInvalidId = computed(() => {
    if (this.aDeviceError()) return 'a-device';
    if (this.aPortError()) return 'a-port';
    if (this.bDeviceError()) return 'b-device';
    if (this.bPortError()) return 'b-port';
    return '';
  });

  // ── Derived: ports of the device being managed ────────────────────────────
  readonly portManagementPorts = computed<Port[]>(() => {
    const dev = this.portManagementDevice();
    if (!dev) return [];
    return this.localDevicePorts()[dev.id] ?? [];
  });

  constructor() {
    effect(() => {
      const ext = this.externalDevicePorts();
      this.localDevicePorts.update((current) => ({ ...current, ...ext }));
    });
    effect(() => {
      const c = this.cable();
      if (!c) return;
      this.saveAttempted.set(false);
      if (c.aSide && c.bSide && !c.id) {
        afterNextRender(() => this.focusAndScrollNameField(), { injector: this.injector });
      }
      this.portManagementDevice.set(null);
      if (c.aSide) {
        this.aDeviceId.set(c.aSide.deviceId);
        this.aPortId.set(c.aSide.portId);
      } else {
        this.aDeviceId.set('');
        this.aPortId.set('');
      }
      if (c.bSide) {
        this.bDeviceId.set(c.bSide.deviceId);
        this.bPortId.set(c.bSide.portId);
      } else {
        this.bDeviceId.set('');
        this.bPortId.set('');
      }
      // Preserve an unset (NULL) type/status when editing an existing
      // connection so we don't silently rewrite it on the next save. New
      // cables still get sensible defaults.
      const isExisting = !!c.id;
      const type = c.type ?? (isExisting ? '' : this.CABLE_TYPES[0]);
      this.cableType.set(type);
      // Whether the key is there, not whether it holds something: a new cable
      // opened from the Unspecified view says status: undefined on purpose, and
      // `??` would read that as "nothing given" and fill in Connected.
      const givenStatus = 'status' in c;
      const fallbackStatus = isExisting ? '' : 'connected';
      this.cableStatus.set(givenStatus ? (c.status ?? '') : fallbackStatus);
      this.cableLabel.set(c.label ?? '');
      if (c.color !== undefined) {
        // Keep a stored color and treat it as a manual choice so a later type
        // change won't overwrite it.
        this.cableColor.set(c.color);
        this.colorManuallySet.set(true);
      } else if (!isExisting && type) {
        // New cable: seed the preset color for the default type.
        this.cableColor.set(CABLE_TYPE_DEFAULT_COLOR[type]);
        this.colorManuallySet.set(false);
      } else {
        this.cableColor.set(undefined);
        this.colorManuallySet.set(false);
      }
      this.cableLength.set(c.length ?? undefined);
      this.cableDescription.set(c.description ?? '');
      this.cableComments.set(c.comments ?? '');
    });
  }

  // ── Cascade handlers ───────────────────────────────────────────────────────

  onDcToggle(id: string, selected: boolean): void {
    if (!selected || id === this.dcId()) return;
    this.aDeviceId.set('');
    this.aPortId.set('');
    this.bDeviceId.set('');
    this.bPortId.set('');
    this.dcChange.emit(id);
  }

  onADeviceChange(value: string): void {
    this.aDeviceId.set(value);
    this.aPortId.set('');
  }

  onBDeviceChange(value: string): void {
    this.bDeviceId.set(value);
    this.bPortId.set('');
  }

  swapSides(): void {
    const aDevice = this.aDeviceId();
    const aPort = this.aPortId();
    this.aDeviceId.set(this.bDeviceId());
    this.aPortId.set(this.bPortId());
    this.bDeviceId.set(aDevice);
    this.bPortId.set(aPort);
  }

  // ── Cable field handlers ─────────────────────────────────────────────────

  onCableTypeChange(value: string): void {
    const type = value as CableType | '';
    this.cableType.set(type);
    // Auto-fill the preset color for the type, unless the user picked one.
    if (!this.colorManuallySet() && type) {
      this.cableColor.set(CABLE_TYPE_DEFAULT_COLOR[type]);
    }
  }

  /** The status as buttons: a radio group, so only the button that becomes
   *  selected has anything to say. */
  onStatusToggle(value: string, selected: boolean): void {
    if (selected) this.cableStatus.set(value as CableStatus | '');
  }

  /** The colour as buttons: a radio group, so only the button that becomes
   *  selected has anything to say. */
  onColorToggle(value: string, selected: boolean): void {
    if (selected) this.onCableColorChange((value || undefined) as CableColor | undefined);
  }

  onCableColorChange(color: CableColor | undefined): void {
    this.colorManuallySet.set(true);
    this.cableColor.set(color);
  }

  // ── Port management ──────────────────────────────────────────────────────────

  openPortManagement(deviceId: string): void {
    const device = this.dcDevices().find((d) => d.id === deviceId);
    if (!device) return;
    this.portManagementDevice.set({ id: device.id, name: device.name });
  }

  closePortManagement(): void {
    this.portManagementDevice.set(null);
  }

  onPortsChanged(ports: Port[]): void {
    const dev = this.portManagementDevice();
    if (!dev) return;
    this.localDevicePorts.update((map) => ({ ...map, [dev.id]: ports }));
    // Clear a chosen port that the edit removed.
    if (this.aDeviceId() === dev.id && !ports.find((p) => p.id === this.aPortId())) {
      this.aPortId.set('');
    }
    if (this.bDeviceId() === dev.id && !ports.find((p) => p.id === this.bPortId())) {
      this.bPortId.set('');
    }
    this.portsUpdated.emit({ deviceId: dev.id, ports });
  }

  // ── Actions ────────────────────────────────────────────────────────────────

  onSave(): void {
    // Save is never dead. Press it on an unfinished form and the form says
    // what is missing, then puts you in the first field that is.
    this.saveAttempted.set(true);
    if (!this.canSave()) {
      const id = this.firstInvalidId();
      // After the render, not before it: the error texts appearing above the
      // field would otherwise push it out of view again.
      if (id) afterNextRender(() => this.focusField(id), { injector: this.injector });
      return;
    }

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

  // ── Focus helpers ──────────────────────────────────────────────────────────

  /** Puts you in a field and brings it on screen, wherever the form scrolled. */
  private focusField(id: string): void {
    const el: HTMLElement | null = this.elRef.nativeElement.querySelector(`#${id}`);
    if (!el) return;
    // One scroll, not two: focusing would park the field just under the sticky
    // header, so we do the scrolling ourselves.
    el.focus({ preventScroll: true });
    el.scrollIntoView({ behavior: 'smooth', block: 'center' });
  }

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

  readonly CABLE_STATUSES = CABLE_STATUSES;

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

  readonly CABLE_COLOR_LABEL = CABLE_COLOR_LABEL;

  readonly CABLE_TYPE_LABEL = CABLE_TYPE_LABEL;

  readonly PORT_TYPE_LABEL = PORT_TYPE_LABEL;
}
