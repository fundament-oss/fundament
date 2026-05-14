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
import { ActivatedRoute, RouterLink } from '@angular/router';
import { firstValueFrom } from 'rxjs';
import { DATACENTER_INFO, DatacenterInfo, RackRow, Room } from '../datacenter.model';
import { RACKS } from '../../racks/rack.model';
import DatacenterApiService from '../datacenter-api.service';
import connectErrorMessage from '../../../connect/error';

interface NativeElementRef {
  nativeElement: { value: string; show?: () => void; hide?: () => void };
}

// ── Component ─────────────────────────────────────────────────────────────────

@Component({
  selector: 'app-datacenter-detail',
  templateUrl: './datacenter-detail.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [RouterLink],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  host: { class: 'flex flex-col bg-white text-slate-900' },
})
export default class DatacenterDetailComponent implements OnInit {
  private readonly route = inject(ActivatedRoute);

  private readonly dcApi = inject(DatacenterApiService);

  readonly dc = computed<DatacenterInfo | undefined>(() => {
    const id = this.route.snapshot.paramMap.get('id') ?? '';
    return DATACENTER_INFO.find((d) => d.id === id);
  });

  // ── Rooms ──────────────────────────────────────────────────────────────────

  readonly mutableRooms = signal<Room[]>([]);

  readonly dcRooms = computed(() => {
    const id = this.route.snapshot.paramMap.get('id') ?? '';
    return this.mutableRooms().filter((r) => r.siteId === id);
  });

  // ── Rack rows ──────────────────────────────────────────────────────────────

  readonly mutableRackRows = signal<RackRow[]>([]);

  rackRowsForRoom(roomId: string): RackRow[] {
    return this.mutableRackRows().filter((rr) => rr.roomId === roomId);
  }

  // ── Racks in this DC ───────────────────────────────────────────────────────

  readonly dcRacks = computed(() => {
    const id = this.route.snapshot.paramMap.get('id') ?? '';
    return RACKS.filter((r) => r.dcId === id);
  });

  // ── Room CRUD ──────────────────────────────────────────────────────────────

  editRoom = signal<Partial<Room> | null>(null);

  deleteRoom = signal<Room | null>(null);

  private readonly roomSheetEl = viewChild<NativeElementRef>('roomSheet');

  private readonly roomModalEl = viewChild<NativeElementRef>('roomModal');

  private readonly fRoomName = viewChild<NativeElementRef>('fRoomName');

  private readonly fRoomFloor = viewChild<NativeElementRef>('fRoomFloor');

  // ── RackRow CRUD ───────────────────────────────────────────────────────────

  editRackRow = signal<Partial<RackRow> | null>(null);

  deleteRackRow = signal<RackRow | null>(null);

  activeRoomId = signal<string>('');

  private readonly rowSheetEl = viewChild<NativeElementRef>('rowSheet');

  private readonly rowModalEl = viewChild<NativeElementRef>('rowModal');

  private readonly fRowName = viewChild<NativeElementRef>('fRowName');

  private readonly fRowX = viewChild<NativeElementRef>('fRowX');

  private readonly fRowY = viewChild<NativeElementRef>('fRowY');

  constructor() {
    effect(() => {
      const el = this.roomSheetEl()?.nativeElement;
      if (this.editRoom() !== null) el?.show?.();
      else el?.hide?.();
    });
    effect(() => {
      const el = this.roomModalEl()?.nativeElement;
      if (this.deleteRoom() !== null) el?.show?.();
      else el?.hide?.();
    });
    effect(() => {
      const el = this.rowSheetEl()?.nativeElement;
      if (this.editRackRow() !== null) el?.show?.();
      else el?.hide?.();
    });
    effect(() => {
      const el = this.rowModalEl()?.nativeElement;
      if (this.deleteRackRow() !== null) el?.show?.();
      else el?.hide?.();
    });
  }

  ngOnInit(): void {
    const siteId = this.route.snapshot.paramMap.get('id') ?? '';
    firstValueFrom(this.dcApi.listRooms(siteId))
      .then((res) => {
        const rooms = res.rooms.map((r) => DatacenterApiService.mapRoom(r));
        this.mutableRooms.set(rooms);
        return Promise.all(
          rooms.map((room) =>
            firstValueFrom(this.dcApi.listRackRows(room.id)).then((rr) =>
              rr.rackRows.map((row) => DatacenterApiService.mapRackRow(row)),
            ),
          ),
        );
      })
      .then((allRows) => {
        this.mutableRackRows.set(allRows.flat());
      })
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));
  }

  // ── Room actions ───────────────────────────────────────────────────────────

  openCreateRoom(): void {
    const dcId = this.route.snapshot.paramMap.get('id') ?? '';
    this.editRoom.set({ id: '', siteId: dcId, name: '', floor: 1 });
  }

  openEditRoom(room: Room): void {
    this.editRoom.set({ ...room });
  }

  closeRoomForm(): void {
    this.editRoom.set(null);
  }

  saveRoom(): void {
    const form = this.editRoom();
    if (!form) return;
    const name = this.fRoomName()?.nativeElement.value ?? '';
    const floor = parseInt(this.fRoomFloor()?.nativeElement.value ?? '1', 10) || 1;
    if (form.id) {
      firstValueFrom(this.dcApi.updateRoom(form.id, name, floor))
        .then(() => {
          const updated: Room = { id: form.id!, siteId: form.siteId!, name, floor };
          this.mutableRooms.update((list) => list.map((r) => (r.id === form.id ? updated : r)));
          this.editRoom.set(null);
        })
        // eslint-disable-next-line no-console
        .catch((err) => console.error(connectErrorMessage(err)));
    } else {
      firstValueFrom(this.dcApi.createRoom(form.siteId!, name, floor))
        .then((res) => {
          const created: Room = { id: res.roomId, siteId: form.siteId!, name, floor };
          this.mutableRooms.update((list) => [...list, created]);
          this.editRoom.set(null);
        })
        // eslint-disable-next-line no-console
        .catch((err) => console.error(connectErrorMessage(err)));
    }
  }

  openDeleteRoom(room: Room): void {
    this.deleteRoom.set(room);
  }

  cancelDeleteRoom(): void {
    this.deleteRoom.set(null);
  }

  confirmDeleteRoom(): void {
    const target = this.deleteRoom();
    if (!target) return;
    firstValueFrom(this.dcApi.deleteRoom(target.id))
      .then(() => {
        this.mutableRooms.update((list) => list.filter((r) => r.id !== target.id));
        this.mutableRackRows.update((list) => list.filter((rr) => rr.roomId !== target.id));
        this.deleteRoom.set(null);
      })
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));
  }

  // ── Rack row actions ───────────────────────────────────────────────────────

  openCreateRackRow(roomId: string): void {
    this.activeRoomId.set(roomId);
    this.editRackRow.set({ id: '', roomId, name: '', positionX: 1, positionY: 1 });
  }

  openEditRackRow(rr: RackRow): void {
    this.activeRoomId.set(rr.roomId);
    this.editRackRow.set({ ...rr });
  }

  closeRackRowForm(): void {
    this.editRackRow.set(null);
  }

  saveRackRow(): void {
    const form = this.editRackRow();
    if (!form) return;
    const name = this.fRowName()?.nativeElement.value ?? '';
    const posX = parseInt(this.fRowX()?.nativeElement.value ?? '1', 10) || 1;
    const posY = parseInt(this.fRowY()?.nativeElement.value ?? '1', 10) || 1;
    if (form.id) {
      firstValueFrom(this.dcApi.updateRackRow(form.id, name, posX, posY))
        .then(() => {
          const updated: RackRow = {
            id: form.id!,
            roomId: form.roomId!,
            name,
            positionX: posX,
            positionY: posY,
          };
          this.mutableRackRows.update((list) =>
            list.map((rr) => (rr.id === form.id ? updated : rr)),
          );
          this.editRackRow.set(null);
        })
        // eslint-disable-next-line no-console
        .catch((err) => console.error(connectErrorMessage(err)));
    } else {
      firstValueFrom(this.dcApi.createRackRow(form.roomId!, name, posX, posY))
        .then((res) => {
          const created: RackRow = {
            id: res.rackRowId,
            roomId: form.roomId!,
            name,
            positionX: posX,
            positionY: posY,
          };
          this.mutableRackRows.update((list) => [...list, created]);
          this.editRackRow.set(null);
        })
        // eslint-disable-next-line no-console
        .catch((err) => console.error(connectErrorMessage(err)));
    }
  }

  openDeleteRackRow(rr: RackRow): void {
    this.deleteRackRow.set(rr);
  }

  cancelDeleteRackRow(): void {
    this.deleteRackRow.set(null);
  }

  confirmDeleteRackRow(): void {
    const target = this.deleteRackRow();
    if (!target) return;
    firstValueFrom(this.dcApi.deleteRackRow(target.id))
      .then(() => {
        this.mutableRackRows.update((list) => list.filter((rr) => rr.id !== target.id));
        this.deleteRackRow.set(null);
      })
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));
  }
}
