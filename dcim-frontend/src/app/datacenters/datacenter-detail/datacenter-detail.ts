import {
  AfterViewInit,
  ChangeDetectionStrategy,
  Component,
  computed,
  CUSTOM_ELEMENTS_SCHEMA,
  effect,
  inject,
  input,
  OnDestroy,
  OnInit,
  output,
  signal,
  TemplateRef,
  untracked,
  viewChild,
} from '@angular/core';
import { Title } from '@angular/platform-browser';
import { toSignal } from '@angular/core/rxjs-interop';
import { ActivatedRoute, ParamMap, Router } from '@angular/router';
import { firstValueFrom, map } from 'rxjs';
import { pageTitle } from '../../shell/page-title';
import parseValidationError from '../../../connect/validation';
import { DatacenterInfo, DatacenterRack, RackRow, Room } from '../datacenter.model';
import DatacenterApiService from '../datacenter-api.service';
import DatacenterListService from '../datacenter-list.service';
import DatacenterNavComponent from '../datacenter-nav';
import SecondaryNavService from '../../shell/secondary-nav.service';
import { viewSlug } from '../../shared/section-views';
import connectErrorMessage from '../../../connect/error';

type InvalidFields = Record<string, string>;

interface NativeElementRef {
  nativeElement: { value: string; show?: () => void; hide?: () => void };
}

// ── Component ─────────────────────────────────────────────────────────────────

@Component({
  selector: 'app-datacenter-detail',
  templateUrl: './datacenter-detail.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [DatacenterNavComponent],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
})
export default class DatacenterDetailComponent implements OnInit, AfterViewInit, OnDestroy {
  private readonly route = inject(ActivatedRoute);

  private readonly router = inject(Router);

  private readonly title = inject(Title);

  private readonly dcApi = inject(DatacenterApiService);

  private readonly secondaryNav = inject(SecondaryNavService);

  /** This section's menu, handed to the shell for as long as the page is open. */
  private readonly secondaryNavTemplate = viewChild.required<TemplateRef<unknown>>('secondaryNav');

  /**
   * Set when this runs inside the layout sheet, which is how it is normally
   * seen. The sheet sits over the data center it is about, so the address is
   * about the page underneath and the caller says which one this is.
   */
  readonly dcSlug = input('');

  /** Closing is the caller's business: over a page it means going back to the
   *  address without the editor, which this component does not know. */
  readonly dismiss = output<void>();

  /** True when this is a sheet over a page rather than a page of its own. */
  readonly inSheet = computed(() => this.dcSlug() !== '');

  ngAfterViewInit(): void {
    // The menu beside the page belongs to whatever is behind the sheet. Taking
    // it over from in here would swap the menu of the page you are looking at.
    if (!this.inSheet()) this.secondaryNav.set(this.secondaryNavTemplate());
  }

  ngOnDestroy(): void {
    if (!this.inSheet()) this.secondaryNav.clear(this.secondaryNavTemplate());
  }

  /**
   * Which data center this is about: its short name is in the address, or is
   * handed in when this runs inside a sheet. Reactive rather than read once,
   * because picking another data center in the menu keeps you here and only
   * swaps the slug.
   */
  private readonly routeSlug = toSignal(
    this.route.paramMap.pipe(map((params: ParamMap) => params.get('slug') ?? '')),
    { initialValue: this.route.snapshot.paramMap.get('slug') ?? '' },
  );

  readonly slug = computed(() => this.dcSlug() || this.routeSlug());

  readonly siteId = computed(
    () => this.allDatacenters().find((dc) => viewSlug(dc.name) === this.slug())?.id ?? '',
  );

  private readonly list = inject(DatacenterListService);

  /** Every data center: the same list the menu beside this page draws, so the
   *  slug resolves without fetching it again. */
  readonly allDatacenters = this.list.datacenters;

  /** True once that list has landed, so a slug can be resolved. */
  readonly sitesLoaded = this.list.loaded;

  /** A different data center in the address means different rooms and racks. */
  private readonly loadForSlug = effect(() => {
    const siteId = this.siteId();
    const listed = this.sitesLoaded();
    untracked(() => {
      if (!listed) return;
      if (!siteId) {
        this.dcLoaded.set(true);
        return;
      }
      this.loadSite(siteId);
      this.loadRoomsAndRacks(siteId);
    });
  });

  readonly dc = signal<DatacenterInfo | undefined>(undefined);

  /** False until the site request settles, so "not found" only shows after loading. */
  readonly dcLoaded = signal(false);

  // ── Rooms ──────────────────────────────────────────────────────────────────

  readonly mutableRooms = signal<Room[]>([]);

  readonly dcRooms = computed(() => this.mutableRooms().filter((r) => r.siteId === this.siteId()));

  // ── Rack rows ──────────────────────────────────────────────────────────────

  readonly mutableRackRows = signal<RackRow[]>([]);

  rackRowsForRoom(roomId: string): RackRow[] {
    return this.mutableRackRows().filter((rr) => rr.roomId === roomId);
  }

  // ── Racks in this DC ───────────────────────────────────────────────────────

  readonly dcRacks = signal<DatacenterRack[]>([]);

  racksForRow(rowId: string): DatacenterRack[] {
    return this.dcRacks().filter((rack) => rack.rowId === rowId);
  }

  // ── Room CRUD ──────────────────────────────────────────────────────────────

  editRoom = signal<Partial<Room> | null>(null);

  roomErrorMessage = signal<string | null>(null);

  roomInvalidFields = signal<InvalidFields>({});

  deleteRoom = signal<Room | null>(null);

  private readonly roomSheetEl = viewChild<NativeElementRef>('roomSheet');

  private readonly roomModalEl = viewChild<NativeElementRef>('roomModal');

  private readonly fRoomName = viewChild<NativeElementRef>('fRoomName');

  private readonly fRoomFloor = viewChild<NativeElementRef>('fRoomFloor');

  // ── RackRow CRUD ───────────────────────────────────────────────────────────

  editRackRow = signal<Partial<RackRow> | null>(null);

  rowErrorMessage = signal<string | null>(null);

  rowInvalidFields = signal<InvalidFields>({});

  deleteRackRow = signal<RackRow | null>(null);

  activeRoomId = signal<string>('');

  private readonly rowSheetEl = viewChild<NativeElementRef>('rowSheet');

  private readonly rowModalEl = viewChild<NativeElementRef>('rowModal');

  private readonly fRowName = viewChild<NativeElementRef>('fRowName');

  private readonly fRowX = viewChild<NativeElementRef>('fRowX');

  private readonly fRowY = viewChild<NativeElementRef>('fRowY');

  // ── Rack CRUD ──────────────────────────────────────────────────────────────

  editRack = signal<Partial<DatacenterRack> | null>(null);

  rackErrorMessage = signal<string | null>(null);

  rackInvalidFields = signal<InvalidFields>({});

  deleteRack = signal<DatacenterRack | null>(null);

  activeRowId = signal<string>('');

  private readonly rackSheetEl = viewChild<NativeElementRef>('rackSheet');

  private readonly rackModalEl = viewChild<NativeElementRef>('rackModal');

  private readonly fRackName = viewChild<NativeElementRef>('fRackName');

  private readonly fRackTotalU = viewChild<NativeElementRef>('fRackTotalU');

  constructor() {
    // The tab says which one you have open, not just which section. Left to the
    // page behind it when this is a sheet: the tab is about where you are, and
    // an overlay has not taken you anywhere.
    effect(() => {
      const name = this.dc()?.name;
      if (name && !this.inSheet()) this.title.setTitle(pageTitle(name));
    });
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
    effect(() => {
      const el = this.rackSheetEl()?.nativeElement;
      if (this.editRack() !== null) el?.show?.();
      else el?.hide?.();
    });
    effect(() => {
      const el = this.rackModalEl()?.nativeElement;
      if (this.deleteRack() !== null) el?.show?.();
      else el?.hide?.();
    });
  }

  ngOnInit(): void {
    // The address carries the short name, so the list of data centers has to
    // land before this page knows whose rooms it is showing.
    this.list.load();
  }

  /** Back to the floor map of this data center: a page steps up to it, a sheet
   *  closes and reveals it. */
  backToFloorMap(): void {
    if (this.inSheet()) {
      this.dismiss.emit();
      return;
    }
    this.router.navigate(['/data-centers', this.slug()]);
  }

  /** The menu beside this page shows every data center; picking one shows its
   *  rooms, because that is the view you are in. */
  openDatacenter(id: string): void {
    const dc = this.allDatacenters().find((d) => d.id === id);
    this.router.navigate(['/data-centers', dc ? viewSlug(dc.name) : '', 'layout']);
  }

  /** The rack as it stands: what is mounted in it, unit by unit. */
  openRack(rack: DatacenterRack): void {
    this.router.navigate(['/racks', rack.id]);
  }

  private loadSite(siteId: string): void {
    firstValueFrom(this.dcApi.getSite(siteId))
      .then((res) => {
        if (res.site) this.dc.set(DatacenterApiService.mapSite(res.site));
      })
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)))
      .finally(() => this.dcLoaded.set(true));
  }

  private loadRoomsAndRacks(siteId: string): void {
    Promise.all([
      firstValueFrom(this.dcApi.listRooms(siteId)),
      firstValueFrom(this.dcApi.listRackRowsBySite(siteId)),
      firstValueFrom(this.dcApi.listRacksBySite(siteId)),
    ])
      .then(([roomsRes, rowsRes, racksRes]) => {
        this.mutableRooms.set(roomsRes.rooms.map((r) => DatacenterApiService.mapRoom(r)));
        this.mutableRackRows.set(
          rowsRes.rackRows.map((row) => DatacenterApiService.mapRackRow(row)),
        );
        this.dcRacks.set(
          racksRes.racks
            .map((summary) => summary.rack)
            .filter((rack): rack is NonNullable<typeof rack> => rack != null)
            .map((rack) => DatacenterApiService.mapRack(rack)),
        );
      })
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));
  }

  // ── Room actions ───────────────────────────────────────────────────────────

  openCreateRoom(): void {
    const dcId = this.route.snapshot.paramMap.get('id') ?? '';
    this.clearRoomErrors();
    this.editRoom.set({ id: '', siteId: dcId, name: '', floor: 1 });
  }

  openEditRoom(room: Room): void {
    this.clearRoomErrors();
    this.editRoom.set({ ...room });
  }

  closeRoomForm(): void {
    this.editRoom.set(null);
    this.clearRoomErrors();
  }

  isRoomFieldInvalid(field: string): boolean {
    return field in this.roomInvalidFields();
  }

  roomFieldError(field: string): string {
    return this.roomInvalidFields()[field] ?? '';
  }

  private clearRoomErrors(): void {
    this.roomInvalidFields.set({});
    this.roomErrorMessage.set(null);
  }

  private handleRoomError(err: unknown): void {
    const { fields, message } = parseValidationError(err);
    this.roomInvalidFields.set(fields);
    this.roomErrorMessage.set(message);
  }

  isRowFieldInvalid(field: string): boolean {
    return field in this.rowInvalidFields();
  }

  rowFieldError(field: string): string {
    return this.rowInvalidFields()[field] ?? '';
  }

  private clearRowErrors(): void {
    this.rowInvalidFields.set({});
    this.rowErrorMessage.set(null);
  }

  private handleRowError(err: unknown): void {
    const { fields, message } = parseValidationError(err);
    this.rowInvalidFields.set(fields);
    this.rowErrorMessage.set(message);
  }

  isRackFieldInvalid(field: string): boolean {
    return field in this.rackInvalidFields();
  }

  rackFieldError(field: string): string {
    return this.rackInvalidFields()[field] ?? '';
  }

  private clearRackErrors(): void {
    this.rackInvalidFields.set({});
    this.rackErrorMessage.set(null);
  }

  private handleRackError(err: unknown): void {
    const { fields, message } = parseValidationError(err);
    this.rackInvalidFields.set(fields);
    this.rackErrorMessage.set(message);
  }

  saveRoom(): void {
    const form = this.editRoom();
    if (!form) return;
    this.clearRoomErrors();
    const name = this.fRoomName()?.nativeElement.value ?? '';
    const floor = parseInt(this.fRoomFloor()?.nativeElement.value ?? '1', 10) || 1;
    if (form.id) {
      firstValueFrom(this.dcApi.updateRoom(form.id, name, floor))
        .then(() => {
          const updated: Room = { id: form.id!, siteId: form.siteId!, name, floor };
          this.mutableRooms.update((list) => list.map((r) => (r.id === form.id ? updated : r)));
          this.editRoom.set(null);
        })
        .catch((err) => this.handleRoomError(err));
    } else {
      firstValueFrom(this.dcApi.createRoom(form.siteId!, name, floor))
        .then((res) => {
          const created: Room = { id: res.roomId, siteId: form.siteId!, name, floor };
          this.mutableRooms.update((list) => [...list, created]);
          this.editRoom.set(null);
        })
        .catch((err) => this.handleRoomError(err));
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
    this.clearRowErrors();
    // Straight from a room the room is known; from the Add menu it is not, so
    // the form starts on the first one and you pick another if you meant that.
    this.activeRoomId.set(roomId || (this.dcRooms()[0]?.id ?? ''));
    this.editRackRow.set({ id: '', roomId, name: '', positionX: 1, positionY: 1 });
  }

  openEditRackRow(rr: RackRow): void {
    this.clearRowErrors();
    this.activeRoomId.set(rr.roomId);
    this.editRackRow.set({ ...rr });
  }

  closeRackRowForm(): void {
    this.clearRowErrors();
    this.editRackRow.set(null);
  }

  saveRackRow(): void {
    const form = this.editRackRow();
    if (!form) return;
    const roomId = form.id ? (form.roomId ?? '') : this.activeRoomId();
    if (!roomId) {
      this.rowInvalidFields.set({ room_id: 'Pick the room this row stands in.' });
      return;
    }
    this.clearRowErrors();
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
        .catch((err) => this.handleRowError(err));
    } else {
      firstValueFrom(this.dcApi.createRackRow(roomId, name, posX, posY))
        .then((res) => {
          const created: RackRow = {
            id: res.rackRowId,
            roomId,
            name,
            positionX: posX,
            positionY: posY,
          };
          this.mutableRackRows.update((list) => [...list, created]);
          this.editRackRow.set(null);
        })
        .catch((err) => this.handleRowError(err));
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

  // ── Rack actions ───────────────────────────────────────────────────────────

  /** The room the rack form is pointing at: picked in the form when the rack is
   *  new, and read from the row it stands in when it already exists. */
  readonly rackRoomId = signal<string>('');

  /** A room holds its own rows, so picking another one lets go of the row. */
  onRackRoomChange(roomId: string): void {
    this.rackRoomId.set(roomId);
    this.activeRowId.set('');
  }

  private roomIdForRow(rowId: string): string {
    return this.mutableRackRows().find((row) => row.id === rowId)?.roomId ?? '';
  }

  openCreateRack(rowId: string): void {
    this.clearRackErrors();
    this.activeRowId.set(rowId);
    this.rackRoomId.set(rowId ? this.roomIdForRow(rowId) : (this.dcRooms()[0]?.id ?? ''));
    this.editRack.set({ id: '', rowId, name: '', totalU: 42 });
  }

  openEditRack(rack: DatacenterRack): void {
    this.clearRackErrors();
    this.activeRowId.set(rack.rowId);
    this.rackRoomId.set(this.roomIdForRow(rack.rowId));
    this.editRack.set({ ...rack });
  }

  closeRackForm(): void {
    this.clearRackErrors();
    this.editRack.set(null);
  }

  saveRack(): void {
    const form = this.editRack();
    if (!form) return;
    // A new rack lands in the row picked in the form; an existing one cannot
    // move, so it keeps the row it was opened from (see the form).
    const rowId = form.id ? (form.rowId ?? '') : this.activeRowId();
    if (!rowId) {
      this.rackInvalidFields.set({ row_id: 'Pick the row this rack stands in.' });
      return;
    }
    this.clearRackErrors();
    const name = this.fRackName()?.nativeElement.value ?? '';
    const totalU = parseInt(this.fRackTotalU()?.nativeElement.value ?? '42', 10) || 42;
    if (form.id) {
      firstValueFrom(this.dcApi.updateRack(form.id, name, totalU))
        .then(() => {
          const updated: DatacenterRack = {
            id: form.id!,
            rowId: form.rowId!,
            name,
            totalU,
            positionInRow: form.positionInRow ?? 0,
          };
          this.dcRacks.update((list) => list.map((r) => (r.id === form.id ? updated : r)));
          this.editRack.set(null);
        })
        .catch((err) => this.handleRackError(err));
    } else {
      firstValueFrom(this.dcApi.createRack(rowId, name, totalU))
        .then((res) => {
          const created: DatacenterRack = {
            id: res.rackId,
            rowId,
            name,
            totalU,
            positionInRow: 0,
          };
          this.dcRacks.update((list) => [...list, created]);
          this.editRack.set(null);
        })
        .catch((err) => this.handleRackError(err));
    }
  }

  openDeleteRack(rack: DatacenterRack): void {
    this.deleteRack.set(rack);
  }

  cancelDeleteRack(): void {
    this.deleteRack.set(null);
  }

  confirmDeleteRack(): void {
    const target = this.deleteRack();
    if (!target) return;
    firstValueFrom(this.dcApi.deleteRack(target.id))
      .then(() => {
        this.dcRacks.update((list) => list.filter((r) => r.id !== target.id));
        this.deleteRack.set(null);
      })
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));
  }
}
