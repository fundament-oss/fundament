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
import { Port, PortType, PORT_TYPE_LABEL, newLocalPortId } from '../cable.model';
import DropdownSyncDirective from '../../shared/dropdown-sync.directive';

@Component({
  selector: 'app-device-ports',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [DropdownSyncDirective],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  templateUrl: './device-ports.html',
})
export default class DevicePortsComponent {
  readonly deviceName = input.required<string>();

  readonly ports = input.required<Port[]>();

  readonly deviceId = input.required<string>();

  // When true, the component's own header and cancel button are hidden;
  // the parent sheet provides navigation back.
  readonly embedded = input(false);

  readonly portsChange = output<Port[]>();

  readonly cancelEdit = output<void>();

  readonly localPorts = signal<Port[]>([]);

  readonly newPortName = signal('');

  readonly newPortType = signal<PortType>('network-interface');

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
    const id = newLocalPortId(this.deviceId());
    const port: Port = {
      id,
      deviceId: this.deviceId(),
      name: this.newPortName().trim(),
      type: this.newPortType(),
    };
    this.localPorts.update((list) => [...list, port]);
    this.newPortName.set('');
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
