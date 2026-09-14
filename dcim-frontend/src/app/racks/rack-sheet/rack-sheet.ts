import {
  ChangeDetectionStrategy,
  Component,
  computed,
  CUSTOM_ELEMENTS_SCHEMA,
  effect,
  inject,
  signal,
  viewChild,
} from '@angular/core';
import { Router } from '@angular/router';
import { firstValueFrom } from 'rxjs';
import DatacenterApiService from '../../datacenters/datacenter-api.service';
import DatacenterListService from '../../datacenters/datacenter-list.service';
import { RackRow, Room } from '../../datacenters/datacenter.model';
import RackApiService from '../rack-api.service';
import RackListService from '../rack-list.service';
import OverlayService from '../../shell/overlay.service';
import parseValidationError from '../../../connect/validation';
import connectErrorMessage from '../../../connect/error';

interface NativeElementRef {
  nativeElement: { value: string; show?: () => void; hide?: () => void };
}

/**
 * The form for one rack, held by the shell rather than by the racks page.
 *
 * Making a rack is not something you do to the page you are on: the add button
 * in the bar offers it from everywhere, and a page that unmounts on navigation
 * cannot hold a sheet that has to outlive it.
 *
 * Because of that the form asks where the rack goes rather than inheriting it.
 * The data center it opens on is the one you were looking at, if you were, and
 * otherwise the first one.
 */
@Component({
  selector: 'app-rack-sheet',
  changeDetection: ChangeDetectionStrategy.OnPush,
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  templateUrl: './rack-sheet.html',
})
export default class RackSheetComponent {
  private readonly rackApi = inject(RackApiService);

  private readonly dcApi = inject(DatacenterApiService);

  private readonly rackList = inject(RackListService);

  private readonly router = inject(Router);

  protected readonly overlays = inject(OverlayService);

  protected readonly datacenters = inject(DatacenterListService).datacenters;

  protected readonly form = this.overlays.rackSheet;

  protected readonly isEdit = computed(() => !!this.form()?.id);

  /** Where it goes: data center, then the hall in it, then the row in that. */
  protected readonly formDcId = signal('');

  protected readonly formRooms = signal<Room[]>([]);

  protected readonly formRows = signal<RackRow[]>([]);

  protected readonly formRoomId = signal('');

  protected readonly formRowId = signal('');

  protected readonly rowsForFormRoom = computed(() =>
    this.formRows().filter((row) => row.roomId === this.formRoomId()),
  );

  protected readonly invalidFields = signal<Record<string, string>>({});

  protected readonly rackErrorMessage = signal<string | null>(null);

  private readonly sheetEl = viewChild<NativeElementRef>('rackSheet');

  private readonly fRackName = viewChild<NativeElementRef>('fRackName');

  private readonly fRackTotalU = viewChild<NativeElementRef>('fRackTotalU');

  constructor() {
    effect(() => {
      const el = this.sheetEl()?.nativeElement;
      if (this.form() !== null) el?.show?.();
      else el?.hide?.();
    });
    effect(() => {
      const rack = this.form();
      if (!rack) return;
      this.invalidFields.set({});
      this.rackErrorMessage.set(null);
      if (rack.id) return;
      const dcId = rack.dcId || this.datacenters()[0]?.id || '';
      this.formDcId.set(dcId);
      this.loadRowOptions(dcId);
    });
  }

  protected isFieldInvalid(field: string): boolean {
    return field in this.invalidFields();
  }

  protected fieldError(field: string): string {
    return this.invalidFields()[field] ?? '';
  }

  /** Picking a data center offers the halls in it, and lands on the first. */
  protected onFormDcToggle(id: string, selected: boolean): void {
    if (!selected || id === this.formDcId()) return;
    this.formDcId.set(id);
    this.formRoomId.set('');
    this.formRowId.set('');
    this.loadRowOptions(id);
  }

  protected onFormRoomToggle(id: string, selected: boolean): void {
    if (!selected || id === this.formRoomId()) return;
    this.formRoomId.set(id);
    this.formRowId.set(this.formRows().find((row) => row.roomId === id)?.id ?? '');
  }

  protected onFormRowToggle(id: string, selected: boolean): void {
    if (selected) this.formRowId.set(id);
  }

  protected close(): void {
    this.overlays.closeRack();
  }

  protected save(): void {
    const form = this.form();
    if (!form) return;
    this.invalidFields.set({});
    this.rackErrorMessage.set(null);
    const name = this.fRackName()?.nativeElement.value ?? '';
    const totalU = parseInt(this.fRackTotalU()?.nativeElement.value ?? '42', 10) || 42;
    const request = form.id
      ? firstValueFrom(this.rackApi.updateRack(form.id, name, totalU)).then(() => form.id ?? '')
      : firstValueFrom(this.rackApi.createRack(name, totalU, this.formRowId())).then(
          (res) => res.rackId ?? '',
        );
    request
      .then((rackId) => {
        this.rackList.load(form.id ? this.rackList.selectedDcId() : this.formDcId());
        // Straight to the new rack, but only if you were in the section: from
        // another page you asked for a rack, not for a change of scene.
        if (!form.id && rackId && this.router.url.startsWith('/racks')) {
          this.router.navigate(['/racks', rackId]);
        }
        this.overlays.closeRack();
      })
      .catch((err) => {
        const { fields, message } = parseValidationError(err);
        this.invalidFields.set(fields);
        this.rackErrorMessage.set(message);
      });
  }

  /** The halls and rows of one data center, so the form can offer a place. */
  private async loadRowOptions(dcId: string): Promise<void> {
    if (!dcId) {
      this.formRooms.set([]);
      this.formRows.set([]);
      return;
    }
    try {
      const [roomsRes, rowsRes] = await Promise.all([
        firstValueFrom(this.dcApi.listRooms(dcId)),
        firstValueFrom(this.dcApi.listRackRowsBySite(dcId)),
      ]);
      const rooms = roomsRes.rooms.map((r) => DatacenterApiService.mapRoom(r));
      const rows = rowsRes.rackRows.map((r) => DatacenterApiService.mapRackRow(r));
      this.formRooms.set(rooms);
      this.formRows.set(rows);
      // Both groups open on their first button, so a new rack always has a
      // place: nothing in this form waits on a choice above it.
      const room = rooms.find((r) => r.id === this.formRoomId()) ?? rooms[0];
      this.formRoomId.set(room?.id ?? '');
      this.formRowId.set(rows.find((r) => r.roomId === room?.id)?.id ?? '');
    } catch (err) {
      // eslint-disable-next-line no-console
      console.error(connectErrorMessage(err));
    }
  }
}
