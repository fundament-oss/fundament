import {
  ChangeDetectionStrategy,
  Component,
  computed,
  CUSTOM_ELEMENTS_SCHEMA,
  effect,
  inject,
  OnInit,
  signal,
  viewChild,
} from '@angular/core';
import { RouterLink, ActivatedRoute } from '@angular/router';
import { firstValueFrom } from 'rxjs';
import DesignFlowWrapperComponent from '../design-flow-wrapper';
import {
  LogicalConnection,
  LogicalConnectionType,
  LogicalDesign,
  LogicalDevice,
  LogicalDeviceLayout,
  LogicalDeviceRole,
  DEVICE_ROLE_COLORS,
} from '../design.model';
import DesignApiService from '../design-api.service';
import connectErrorMessage from '../../../connect/error';

interface NativeElementRef {
  nativeElement: { value: string; show?: () => void; hide?: () => void };
}

interface NativeElementRef {
  nativeElement: { value: string; show?: () => void; hide?: () => void };
}

const ALL_ROLES: LogicalDeviceRole[] = [
  'Compute',
  'ToR',
  'Spine',
  'Core',
  'PDU',
  'Patch Panel',
  'Storage',
  'Firewall',
  'Load Balancer',
  'Console Server',
  'Cable Manager',
  'Adapter',
];

@Component({
  selector: 'app-design-detail',
  templateUrl: './design-detail.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [RouterLink, DesignFlowWrapperComponent],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  host: { class: 'flex flex-col overflow-hidden', style: 'height: calc(100dvh - 4.25rem)' },
})
export default class DesignDetailComponent implements OnInit {
  private readonly route = inject(ActivatedRoute);

  private readonly designApi = inject(DesignApiService);

  readonly designId = this.route.snapshot.paramMap.get('id') ?? '';

  // ── Mutable state ──────────────────────────────────────────────────────────
  readonly mutableDesign = signal<LogicalDesign | undefined>(undefined);

  readonly mutableDevices = signal<LogicalDevice[]>([]);

  readonly mutableConnections = signal<LogicalConnection[]>([]);

  readonly mutableLayouts = signal<LogicalDeviceLayout[]>([]);

  readonly design = computed<LogicalDesign | undefined>(() => this.mutableDesign());

  readonly devices = computed(() =>
    this.mutableDevices().filter((d) => d.designId === this.designId),
  );

  readonly connections = computed(() =>
    this.mutableConnections().filter((c) => c.designId === this.designId),
  );

  readonly layouts = computed(() => this.mutableLayouts());

  // ── Selection ──────────────────────────────────────────────────────────────
  selectedDeviceId = signal<string | null>(null);

  readonly selectedDevice = computed(() => {
    const id = this.selectedDeviceId();
    return id ? (this.mutableDevices().find((d) => d.id === id) ?? null) : null;
  });

  readonly selectedDeviceConnections = computed(() => {
    const id = this.selectedDeviceId();
    if (!id) return [];
    return this.mutableConnections().filter(
      (c) => c.designId === this.designId && (c.sourceDeviceId === id || c.targetDeviceId === id),
    );
  });

  // ── Device CRUD state ──────────────────────────────────────────────────────
  editDevice = signal<Partial<LogicalDevice> | null>(null);

  deleteDevice = signal<LogicalDevice | null>(null);

  private readonly deviceSheetEl = viewChild<NativeElementRef>('deviceSheet');

  private readonly deviceModalEl = viewChild<NativeElementRef>('deviceModal');

  private readonly fDeviceName = viewChild<NativeElementRef>('fDeviceName');

  private readonly fDeviceRole = viewChild<NativeElementRef>('fDeviceRole');

  // ── Connection CRUD state ──────────────────────────────────────────────────
  editConnection = signal<Partial<LogicalConnection> | null>(null);

  deleteConnection = signal<LogicalConnection | null>(null);

  private readonly connSheetEl = viewChild<NativeElementRef>('connSheet');

  private readonly connModalEl = viewChild<NativeElementRef>('connModal');

  private readonly fConnSrcDevice = viewChild<NativeElementRef>('fConnSrcDevice');

  private readonly fConnSrcPort = viewChild<NativeElementRef>('fConnSrcPort');

  private readonly fConnTgtDevice = viewChild<NativeElementRef>('fConnTgtDevice');

  private readonly fConnTgtPort = viewChild<NativeElementRef>('fConnTgtPort');

  private readonly fConnType = viewChild<NativeElementRef>('fConnType');

  readonly allRoles = ALL_ROLES;

  constructor() {
    effect(() => {
      const el = this.deviceSheetEl()?.nativeElement;
      if (this.editDevice() !== null) el?.show?.();
      else el?.hide?.();
    });
    effect(() => {
      const el = this.deviceModalEl()?.nativeElement;
      if (this.deleteDevice() !== null) el?.show?.();
      else el?.hide?.();
    });
    effect(() => {
      const el = this.connSheetEl()?.nativeElement;
      if (this.editConnection() !== null) el?.show?.();
      else el?.hide?.();
    });
    effect(() => {
      const el = this.connModalEl()?.nativeElement;
      if (this.deleteConnection() !== null) el?.show?.();
      else el?.hide?.();
    });
  }

  ngOnInit(): void {
    firstValueFrom(this.designApi.listDevices(this.designId))
      .then((res) => this.mutableDevices.set(res.devices.map((d) => DesignApiService.mapDevice(d))))
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));

    firstValueFrom(this.designApi.listConnections(this.designId))
      .then((res) =>
        this.mutableConnections.set(res.connections.map((c) => DesignApiService.mapConnection(c))),
      )
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));

    firstValueFrom(this.designApi.getLayout(this.designId))
      .then((res) =>
        this.mutableLayouts.set(res.positions.map((l) => DesignApiService.mapLayout(l))),
      )
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));

    firstValueFrom(this.designApi.listDesigns())
      .then((res) => {
        const found = res.designs.find((d) => d.id === this.designId);
        if (found) this.mutableDesign.set(DesignApiService.mapDesign(found));
      })
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));
  }

  // ── Device actions ─────────────────────────────────────────────────────────

  openAddDevice(): void {
    this.editDevice.set({ id: '', designId: this.designId, name: '', role: 'Compute' });
  }

  openEditDevice(device: LogicalDevice): void {
    this.editDevice.set({ ...device });
  }

  closeDeviceForm(): void {
    this.editDevice.set(null);
  }

  saveDevice(): void {
    const form = this.editDevice();
    if (!form) return;
    const name = this.fDeviceName()?.nativeElement.value ?? '';
    const role = (this.fDeviceRole()?.nativeElement.value ?? 'Compute') as LogicalDeviceRole;
    if (form.id) {
      firstValueFrom(this.designApi.updateDevice(form.id, name, role))
        .then(() => {
          const updated: LogicalDevice = {
            id: form.id!,
            designId: form.designId ?? this.designId,
            name,
            role,
          };
          this.mutableDevices.update((list) => list.map((d) => (d.id === form.id ? updated : d)));
          this.editDevice.set(null);
        })
        // eslint-disable-next-line no-console
        .catch((err) => console.error(connectErrorMessage(err)));
    } else {
      firstValueFrom(this.designApi.createDevice(this.designId, name, role))
        .then((res) => {
          const created: LogicalDevice = { id: res.deviceId, designId: this.designId, name, role };
          this.mutableDevices.update((list) => [...list, created]);
          this.mutableLayouts.update((list) => [
            ...list,
            { deviceId: res.deviceId, x: 200 + Math.random() * 200, y: 200 + Math.random() * 200 },
          ]);
          this.editDevice.set(null);
        })
        // eslint-disable-next-line no-console
        .catch((err) => console.error(connectErrorMessage(err)));
    }
  }

  openDeleteDevice(device: LogicalDevice): void {
    this.deleteDevice.set(device);
    this.editDevice.set(null);
  }

  cancelDeleteDevice(): void {
    this.deleteDevice.set(null);
  }

  confirmDeleteDevice(): void {
    const target = this.deleteDevice();
    if (!target) return;
    firstValueFrom(this.designApi.deleteDevice(target.id))
      .then(() => {
        this.mutableDevices.update((list) => list.filter((d) => d.id !== target.id));
        this.mutableConnections.update((list) =>
          list.filter((c) => c.sourceDeviceId !== target.id && c.targetDeviceId !== target.id),
        );
        this.mutableLayouts.update((list) => list.filter((l) => l.deviceId !== target.id));
        if (this.selectedDeviceId() === target.id) this.selectedDeviceId.set(null);
        this.deleteDevice.set(null);
      })
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));
  }

  // ── Connection actions ─────────────────────────────────────────────────────

  openAddConnection(): void {
    this.editConnection.set({
      id: '',
      designId: this.designId,
      sourceDeviceId: this.selectedDeviceId() ?? '',
      sourcePortRole: '',
      targetDeviceId: '',
      targetPortRole: '',
      connectionType: 'network',
    });
  }

  openEditConnection(conn: LogicalConnection): void {
    this.editConnection.set({ ...conn });
  }

  closeConnForm(): void {
    this.editConnection.set(null);
  }

  saveConnection(): void {
    const form = this.editConnection();
    if (!form) return;
    const srcDeviceId = this.fConnSrcDevice()?.nativeElement.value ?? '';
    const srcPort = this.fConnSrcPort()?.nativeElement.value ?? '';
    const tgtDeviceId = this.fConnTgtDevice()?.nativeElement.value ?? '';
    const tgtPort = this.fConnTgtPort()?.nativeElement.value ?? '';
    const connType = (this.fConnType()?.nativeElement.value ?? 'network') as LogicalConnectionType;
    const conn: LogicalConnection = {
      id: form.id || '',
      designId: this.designId,
      sourceDeviceId: srcDeviceId,
      sourcePortRole: srcPort,
      targetDeviceId: tgtDeviceId,
      targetPortRole: tgtPort,
      connectionType: connType,
    };
    if (form.id) {
      firstValueFrom(this.designApi.updateConnection(conn))
        .then(() => {
          this.mutableConnections.update((list) => list.map((c) => (c.id === form.id ? conn : c)));
          this.editConnection.set(null);
        })
        // eslint-disable-next-line no-console
        .catch((err) => console.error(connectErrorMessage(err)));
    } else {
      firstValueFrom(this.designApi.createConnection(conn))
        .then((res) => {
          this.mutableConnections.update((list) => [...list, { ...conn, id: res.connectionId }]);
          this.editConnection.set(null);
        })
        // eslint-disable-next-line no-console
        .catch((err) => console.error(connectErrorMessage(err)));
    }
  }

  openDeleteConnection(conn: LogicalConnection): void {
    this.deleteConnection.set(conn);
    this.editConnection.set(null);
  }

  cancelDeleteConnection(): void {
    this.deleteConnection.set(null);
  }

  confirmDeleteConnection(): void {
    const target = this.deleteConnection();
    if (!target) return;
    firstValueFrom(this.designApi.deleteConnection(target.id))
      .then(() => {
        this.mutableConnections.update((list) => list.filter((c) => c.id !== target.id));
        this.deleteConnection.set(null);
      })
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));
  }

  // ── Design status transitions ──────────────────────────────────────────────

  activateDesign(): void {
    firstValueFrom(this.designApi.updateDesign(this.designId, 'active'))
      .then(() => this.mutableDesign.update((d) => (d ? { ...d, status: 'active' as const } : d)))
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));
  }

  archiveDesign(): void {
    firstValueFrom(this.designApi.updateDesign(this.designId, 'archived'))
      .then(() => this.mutableDesign.update((d) => (d ? { ...d, status: 'archived' as const } : d)))
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));
  }

  // ── Layout persistence ─────────────────────────────────────────────────────

  onLayoutChanged(layouts: LogicalDeviceLayout[]): void {
    this.mutableLayouts.set(layouts);

    firstValueFrom(this.designApi.saveLayout(this.designId, layouts)).catch((err) =>
      // eslint-disable-next-line no-console
      console.error(connectErrorMessage(err)),
    );
  }

  // ── Helpers ────────────────────────────────────────────────────────────────

  deviceName(id: string): string {
    return this.mutableDevices().find((d) => d.id === id)?.name ?? id;
  }

  readonly roleColor = (role: LogicalDeviceRole): string =>
    DEVICE_ROLE_COLORS[role]?.text ?? '#475569';

  readonly roleBg = (role: LogicalDeviceRole): string => DEVICE_ROLE_COLORS[role]?.bg ?? '#f8fafc';

  readonly roleBorder = (role: LogicalDeviceRole): string =>
    DEVICE_ROLE_COLORS[role]?.border ?? '#94a3b8';

  readonly connTypeLabel = (type: LogicalConnectionType): string => {
    const connMap: Record<LogicalConnectionType, string> = {
      network: 'Network',
      power: 'Power',
      console: 'Console',
    };
    return connMap[type];
  };

  readonly connTypeBadgeClass = (type: LogicalConnectionType): string => {
    const connMap: Record<LogicalConnectionType, string> = {
      network: 'bg-blue-50 text-blue-700',
      power: 'bg-amber-50 text-amber-700',
      console: 'bg-slate-100 text-slate-600',
    };
    return connMap[type];
  };

  readonly statusBadgeClass = (status: string): string => {
    const statusMap: Record<string, string> = {
      draft: 'bg-slate-100 text-slate-600',
      active: 'bg-green-50 text-green-700',
      archived: 'bg-amber-50 text-amber-700',
    };
    return statusMap[status] ?? 'bg-slate-100 text-slate-600';
  };
}
