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
import { firstValueFrom } from 'rxjs';
import { RackSlotType } from '../../../generated/v1/common_pb';
import DatacenterListService from '../../datacenters/datacenter-list.service';
import InventoryApiService from '../../inventory/inventory-api.service';
import PlacementApiService from '../../inventory/placement-api.service';
import RackListService from '../rack-list.service';
import OverlayService from '../../shell/overlay.service';
import parseValidationError from '../../../connect/validation';
import connectErrorMessage from '../../../connect/error';

interface NativeElementRef {
  nativeElement: { value: string; show?: () => void; hide?: () => void };
}

interface AssetOption {
  id: string;
  label: string;
}

/**
 * The form that puts an asset in a rack, held by the shell rather than by the
 * racks page.
 *
 * Placing an asset is not something you do to the page you are on: the add
 * button in the bar offers it from everywhere, and a page that unmounts on
 * navigation cannot hold a sheet that has to outlive it.
 *
 * Because of that the form asks where the asset goes rather than inheriting it.
 * Opened from a free slot it starts on that rack and that unit; opened from the
 * bar it starts on the rack you were looking at, and otherwise on the first one
 * there is. Every one of those is a starting point, not an answer.
 */
@Component({
  selector: 'app-placement-sheet',
  changeDetection: ChangeDetectionStrategy.OnPush,
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  templateUrl: './placement-sheet.html',
})
export default class PlacementSheetComponent {
  private readonly placementApi = inject(PlacementApiService);

  private readonly inventoryApi = inject(InventoryApiService);

  private readonly rackList = inject(RackListService);

  protected readonly overlays = inject(OverlayService);

  protected readonly datacenters = inject(DatacenterListService).datacenters;

  protected readonly form = this.overlays.placementSheet;

  /** Where it goes: the data center first, then a rack in it. */
  protected readonly formDcId = signal('');

  protected readonly formRackId = signal('');

  protected readonly slotType = signal<RackSlotType>(RackSlotType.UNIT);

  protected readonly slotTypes: { value: RackSlotType; label: string }[] = [
    { value: RackSlotType.UNIT, label: 'Unit' },
    { value: RackSlotType.POWER, label: 'Power' },
    { value: RackSlotType.ZERO_U, label: 'Zero-U' },
  ];

  protected readonly assetOptions = signal<AssetOption[]>([]);

  protected readonly invalidFields = signal<Record<string, string>>({});

  protected readonly errorMessage = signal<string | null>(null);

  /** The racks of the chosen data center, by name, because that is what the
   *  field asks for and what everybody calls them. */
  protected readonly racksForDc = computed(() =>
    [...this.rackList.racks().filter((rack) => rack.dcId === this.formDcId())].sort((a, b) =>
      a.name.localeCompare(b.name),
    ),
  );

  protected readonly selectedRack = computed(() =>
    this.racksForDc().find((rack) => rack.id === this.formRackId()),
  );

  /** A rack unit cannot be higher than the rack is tall, and until one is
   *  chosen there is nothing to bound it by. */
  protected readonly maxUnit = computed(() => this.selectedRack()?.totalU ?? 1);

  private readonly sheetEl = viewChild<NativeElementRef>('placementSheet');

  private readonly fAsset = viewChild<NativeElementRef>('fAsset');

  private readonly fRack = viewChild<NativeElementRef>('fRack');

  private readonly fRackUnit = viewChild<NativeElementRef>('fRackUnit');

  constructor() {
    effect(() => {
      const el = this.sheetEl()?.nativeElement;
      if (this.form() !== null) el?.show?.();
      else el?.hide?.();
    });
    effect(() => {
      const draft = this.form();
      if (!draft) return;
      this.invalidFields.set({});
      this.errorMessage.set(null);
      this.slotType.set(RackSlotType.UNIT);
      // The rack you came from, else the one the section is on, else the first
      // data center there is. The form shows all three the same way, so it
      // never matters to the reader which of them answered.
      const dcId = draft.dcId || this.rackList.selectedDcId() || this.datacenters()[0]?.id || '';
      this.formDcId.set(dcId);
      this.rackList.load(dcId);
      this.formRackId.set(draft.rackId || this.rackList.openRackId() || '');
      this.loadAssets();
    });
    effect(() => {
      // A data center with no rack chosen in it lands on its first, so the
      // unit field below always has something to measure itself against.
      const racks = this.racksForDc();
      if (racks.length && !racks.some((rack) => rack.id === this.formRackId())) {
        this.formRackId.set(racks[0].id);
      }
    });
  }

  protected isFieldInvalid(field: string): boolean {
    return field in this.invalidFields();
  }

  protected fieldError(field: string): string {
    return this.invalidFields()[field] ?? '';
  }

  /** Picking a data center offers the racks in it, and lands on the first. */
  protected onDcToggle(id: string, selected: boolean): void {
    if (!selected || id === this.formDcId()) return;
    this.formDcId.set(id);
    this.formRackId.set('');
    this.rackList.load(id);
  }

  protected onRackChange(id: string): void {
    this.formRackId.set(id);
  }

  /** The slot type as buttons: a radio group, so only the button that becomes
   *  selected has anything to say. */
  protected onSlotTypeToggle(value: RackSlotType, selected: boolean): void {
    if (selected) this.slotType.set(value);
  }

  protected close(): void {
    this.overlays.closePlacement();
  }

  protected save(): void {
    if (!this.form()) return;
    this.invalidFields.set({});
    this.errorMessage.set(null);
    const assetId = this.fAsset()?.nativeElement.value ?? '';
    const rackId = this.fRack()?.nativeElement.value || this.formRackId();
    const rackUnitStart = parseInt(this.fRackUnit()?.nativeElement.value ?? '0', 10) || 0;
    firstValueFrom(
      this.placementApi.createPlacement(assetId, rackId, rackUnitStart, this.slotType()),
    )
      .then(() => {
        // The rack that gained a device is now wrong in the menu and on the
        // page, and both read the same service.
        this.rackList.load(this.formDcId());
        this.overlays.closePlacement();
      })
      .catch((err) => {
        const { fields, message } = parseValidationError(err);
        this.invalidFields.set(fields);
        this.errorMessage.set(message);
      });
  }

  /** Every asset there is, so the field can offer one by its tag. */
  private loadAssets(): void {
    firstValueFrom(
      this.inventoryApi.listAssets({ status: 'all', category: 'all', sortDirection: 'asc' }),
    )
      .then((res) =>
        this.assetOptions.set(res.assets.map((a) => ({ id: a.id, label: a.assetTag || a.id }))),
      )
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));
  }
}
