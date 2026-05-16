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
import { Port, PortType, PORT_TYPE_LABEL } from '../cable.model';

@Component({
  selector: 'app-device-ports',
  changeDetection: ChangeDetectionStrategy.OnPush,
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  templateUrl: './device-ports.html',
})
export default class DevicePortsComponent {
  readonly deviceName = input.required<string>();

  readonly ports = input.required<Port[]>();

  readonly deviceId = input.required<string>();

  readonly portsChange = output<Port[]>();

  readonly cancelEdit = output<void>();

  readonly localPorts = signal<Port[]>([]);

  readonly newPortName = signal('');

  readonly newPortType = signal<PortType>('network-interface');

  readonly newPortLabel = signal('');

  readonly canAddPort = computed(() => this.newPortName().trim().length > 0);

  readonly PORT_TYPES: { value: PortType; label: string }[] = [
    { value: 'network-interface', label: 'Network Interface' },
    { value: 'console-port', label: 'Console Port' },
    { value: 'console-server-port', label: 'Console Server Port' },
    { value: 'power-port', label: 'Power Port' },
    { value: 'power-outlet', label: 'Power Outlet' },
  ];

  readonly PORT_TYPE_LABEL = PORT_TYPE_LABEL;

  constructor() {
    effect(() => {
      this.localPorts.set([...this.ports()]);
    });
  }

  addPort(): void {
    if (!this.canAddPort()) return;
    const id = `p-${this.deviceId()}-${Date.now().toString(36)}`;
    const port: Port = {
      id,
      deviceId: this.deviceId(),
      name: this.newPortName().trim(),
      type: this.newPortType(),
      label: this.newPortLabel().trim() || undefined,
    };
    this.localPorts.update((list) => [...list, port]);
    this.newPortName.set('');
    this.newPortLabel.set('');
  }

  removePort(portId: string): void {
    this.localPorts.update((list) => list.filter((p) => p.id !== portId));
  }

  onSave(): void {
    this.portsChange.emit(this.localPorts());
  }

  onCancel(): void {
    this.cancelEdit.emit();
  }
}
