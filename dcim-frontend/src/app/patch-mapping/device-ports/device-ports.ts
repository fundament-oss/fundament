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
  viewChild,
} from '@angular/core';
import { Port, PortType, PORT_TYPE_LABEL, newLocalPortId } from '../cable.model';

interface NativeElementRef {
  nativeElement: { show?: () => void; hide?: () => void };
}

@Component({
  selector: 'app-device-ports',
  changeDetection: ChangeDetectionStrategy.OnPush,
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  templateUrl: './device-ports.html',
  // A split view hides the back button while both panes are on screen, because
  // the menu beside you is the way back. In a sheet there is no menu beside
  // you, so here it always shows.
  styles: `
    :host {
      --context-back-button-display: flex;
    }
  `,
})
export default class DevicePortsComponent {
  private readonly elRef = inject(ElementRef);

  private readonly injector = inject(Injector);

  readonly deviceName = input.required<string>();

  readonly ports = input.required<Port[]>();

  readonly deviceId = input.required<string>();

  /**
   * Every add and every removal, as the list that came out of it. There is no
   * save step: a port you add exists, a port you remove is gone. That is why
   * removing asks first.
   */
  readonly portsChange = output<Port[]>();

  readonly back = output<void>();

  readonly newPortName = signal('');

  readonly newPortType = signal<PortType>('network-interface');

  /** Set by pressing Add port: a name you have not typed yet is not wrong. */
  readonly addAttempted = signal(false);

  /** Names the device, so the page says what it is about on its own. */
  readonly title = computed(() => `Ports for ${this.deviceName()}`);

  readonly nameError = computed(() =>
    this.addAttempted() && !this.newPortName().trim() ? 'Give the port a name.' : '',
  );

  /** The port the confirmation is about, null when nothing is being removed. */
  readonly removing = signal<Port | null>(null);

  private readonly removeModalEl = viewChild<NativeElementRef>('removeModal');

  constructor() {
    effect(() => {
      const el = this.removeModalEl()?.nativeElement;
      if (this.removing()) el?.show?.();
      else el?.hide?.();
    });
  }

  readonly PORT_TYPES: { value: PortType; label: string }[] = [
    { value: 'network-interface', label: 'Network Interface' },
    { value: 'console-port', label: 'Console Port' },
    { value: 'console-server-port', label: 'Console Server Port' },
    { value: 'power-port', label: 'Power Port' },
    { value: 'power-outlet', label: 'Power Outlet' },
  ];

  readonly PORT_TYPE_LABEL = PORT_TYPE_LABEL;

  addPort(): void {
    this.addAttempted.set(true);
    const name = this.newPortName().trim();
    if (!name) {
      afterNextRender(() => this.focusNameField(), { injector: this.injector });
      return;
    }
    const port: Port = {
      id: newLocalPortId(this.deviceId()),
      deviceId: this.deviceId(),
      name,
      type: this.newPortType(),
    };
    this.portsChange.emit([...this.ports(), port]);
    this.newPortName.set('');
    this.addAttempted.set(false);
    this.focusNameField();
  }

  askRemove(port: Port): void {
    this.removing.set(port);
  }

  cancelRemove(): void {
    this.removing.set(null);
  }

  confirmRemove(): void {
    const port = this.removing();
    if (!port) return;
    this.portsChange.emit(this.ports().filter((p) => p.id !== port.id));
    this.removing.set(null);
  }

  private focusNameField(): void {
    const el: HTMLElement | null = this.elRef.nativeElement.querySelector('#new-port-name');
    el?.focus();
  }
}
