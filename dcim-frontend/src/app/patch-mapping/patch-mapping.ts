import {
  ChangeDetectionStrategy,
  Component,
  CUSTOM_ELEMENTS_SCHEMA,
  effect,
  ElementRef,
  inject,
  signal,
  viewChild,
} from '@angular/core';
import { firstValueFrom } from 'rxjs';
import DcSelectorComponent from '../shared/dc-selector';
import PatchMappingFlowWrapperComponent from './patch-mapping-flow-wrapper';
import { MOCK_PHYSICAL_CONNECTIONS, PhysicalConnection } from './patch-mapping.model';
import PatchMappingApiService from './patch-mapping-api.service';
import connectErrorMessage from '../../connect/error';

@Component({
  selector: 'app-patch-mapping',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [DcSelectorComponent, PatchMappingFlowWrapperComponent],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  host: {
    class: 'flex flex-col overflow-hidden bg-white text-slate-900',
    style: 'height: calc(100dvh - 4.25rem)',
  },
  templateUrl: './patch-mapping.html',
})
export default class PatchMappingComponent {
  private readonly patchApi = inject(PatchMappingApiService);

  readonly selectedDcId = signal('ams-01');

  // ── Mutable connection list ────────────────────────────────────────────────
  readonly mutableConnections = signal([...MOCK_PHYSICAL_CONNECTIONS]);

  // ── CRUD state — null = closed, object = open (new or edit) ───────────────
  editConnection = signal<Partial<PhysicalConnection> | null>(null);

  deleteConnection = signal<PhysicalConnection | null>(null);

  connectionsVisible = signal(false);

  private readonly connSheetEl = viewChild<ElementRef>('connSheet');

  private readonly deleteModalEl = viewChild<ElementRef>('deleteModal');

  private readonly fSrcDevice = viewChild<ElementRef>('fSrcDevice');

  private readonly fSrcPort = viewChild<ElementRef>('fSrcPort');

  private readonly fTgtDevice = viewChild<ElementRef>('fTgtDevice');

  private readonly fTgtPort = viewChild<ElementRef>('fTgtPort');

  constructor() {
    effect(() => {
      const el = this.connSheetEl()?.nativeElement as { show?: () => void; hide?: () => void };
      if (this.editConnection() !== null) el?.show?.();
      else el?.hide?.();
    });
    effect(() => {
      const el = this.deleteModalEl()?.nativeElement as { show?: () => void; hide?: () => void };
      if (this.deleteConnection() !== null) el?.show?.();
      else el?.hide?.();
    });
  }

  // ── CRUD actions ───────────────────────────────────────────────────────────

  openAddConnection(): void {
    this.editConnection.set({ id: '', dcId: this.selectedDcId() });
  }

  openEditConnection(conn: PhysicalConnection): void {
    this.editConnection.set({ ...conn });
  }

  closeConnForm(): void {
    this.editConnection.set(null);
  }

  saveConnection(): void {
    const form = this.editConnection();
    if (!form) return;
    const srcDevice = (this.fSrcDevice()?.nativeElement as HTMLInputElement)?.value ?? '';
    const srcPort = (this.fSrcPort()?.nativeElement as HTMLInputElement)?.value ?? '';
    const tgtDevice = (this.fTgtDevice()?.nativeElement as HTMLInputElement)?.value ?? '';
    const tgtPort = (this.fTgtPort()?.nativeElement as HTMLInputElement)?.value ?? '';
    if (!srcDevice || !srcPort || !tgtDevice || !tgtPort) return;

    if (form.id) {
      firstValueFrom(this.patchApi.updatePhysicalConnection(form.id, ''))
        .then(() => {
          this.mutableConnections.update((list) =>
            list.map((c) =>
              c.id === form.id
                ? {
                    ...c,
                    sourceDeviceLabel: srcDevice,
                    sourcePortName: srcPort,
                    targetDeviceLabel: tgtDevice,
                    targetPortName: tgtPort,
                  }
                : c,
            ),
          );
          this.editConnection.set(null);
        })
        // eslint-disable-next-line no-console
        .catch((err) => console.error(connectErrorMessage(err)));
    } else {
      const srcPlacement = srcDevice.toLowerCase().replace(/\s+/g, '-');
      const tgtPlacement = tgtDevice.toLowerCase().replace(/\s+/g, '-');
      firstValueFrom(
        this.patchApi.createPhysicalConnection(srcPlacement, srcPort, tgtPlacement, tgtPort),
      )
        .then((res) => {
          const created = PatchMappingApiService.mapConnection(res.connection!);
          const newConn: PhysicalConnection = {
            ...created,
            dcId: this.selectedDcId(),
            sourceDeviceLabel: srcDevice,
            targetDeviceLabel: tgtDevice,
          };
          this.mutableConnections.update((list) => [...list, newConn]);
          this.editConnection.set(null);
        })
        // eslint-disable-next-line no-console
        .catch((err) => console.error(connectErrorMessage(err)));
    }
  }

  openDeleteConnection(conn: PhysicalConnection): void {
    this.deleteConnection.set(conn);
    this.editConnection.set(null);
  }

  cancelDeleteConnection(): void {
    this.deleteConnection.set(null);
  }

  confirmDeleteConnection(): void {
    const target = this.deleteConnection();
    if (!target) return;
    firstValueFrom(this.patchApi.deletePhysicalConnection(target.id))
      .then(() => {
        this.mutableConnections.update((list) => list.filter((c) => c.id !== target.id));
        this.deleteConnection.set(null);
      })
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));
  }

  dcConnections(): PhysicalConnection[] {
    return this.mutableConnections().filter((c) => c.dcId === this.selectedDcId());
  }
}
