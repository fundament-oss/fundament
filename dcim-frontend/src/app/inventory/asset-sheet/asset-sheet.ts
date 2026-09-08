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
  signal,
  untracked,
  viewChild,
} from '@angular/core';
import { firstValueFrom } from 'rxjs';
import InventoryApiService from '../inventory-api.service';
import { Asset, AssetStatus, CatalogEntry } from '../inventory';
import { ASSET_STATUSES } from '../asset-status';
import CatalogApiService from '../../catalog/catalog-api.service';
import PlacementApiService, { RackOption } from '../placement-api.service';
import InventoryStatsService from '../inventory-stats.service';
import OverlayService from '../../shell/overlay.service';
import { RackSlotType } from '../../../generated/v1/common_pb';
import parseValidationError from '../../../connect/validation';
import connectErrorMessage from '../../../connect/error';

interface NativeElementRef {
  nativeElement: { value: string; show?: () => void; hide?: () => void };
}

/**
 * The form for one asset, held by the shell rather than by a page.
 *
 * Two pages used to carry their own copy of it: the inventory list and the page
 * of one asset. That is one form written twice, and neither could be opened
 * from anywhere else. This is the single one, and it asks for everything it
 * needs rather than reading it off the page underneath: the catalog it picks a
 * device from, and the racks it can stand in.
 */
@Component({
  selector: 'app-asset-sheet',
  changeDetection: ChangeDetectionStrategy.OnPush,
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  templateUrl: './asset-sheet.html',
})
export default class AssetSheetComponent {
  private readonly inventoryApi = inject(InventoryApiService);

  private readonly catalogApi = inject(CatalogApiService);

  private readonly placementApi = inject(PlacementApiService);

  private readonly stats = inject(InventoryStatsService);

  protected readonly overlays = inject(OverlayService);

  private readonly injector = inject(Injector);

  protected readonly form = this.overlays.assetSheet;

  protected readonly isEdit = computed(() => !!this.form()?.id);

  /** Every product, to pick what this asset is. */
  protected readonly catalog = signal<CatalogEntry[]>([]);

  private catalogById = new Map<string, CatalogEntry>();

  /** Every rack, to pick where it stands. */
  protected readonly racks = signal<RackOption[]>([]);

  protected readonly assetDeviceId = signal('');

  protected readonly assetStatus = signal<AssetStatus>('available');

  protected readonly assetRackId = signal('');

  protected readonly assetSlotType = signal<RackSlotType | ''>('');

  /** Where the asset stands now, so a move knows what it is moving and the
   *  unit field can show the unit it is in. */
  protected readonly placement = signal<{
    id: string;
    rackId: string;
    unit: number;
    slotType: RackSlotType;
  } | null>(null);

  protected readonly pickedLocation = signal('');

  protected readonly invalidFields = signal<Record<string, string>>({});

  protected readonly formErrorMessage = signal<string | null>(null);

  protected readonly statuses = ASSET_STATUSES;

  protected readonly slotTypes: { value: RackSlotType; label: string }[] = [
    { value: RackSlotType.UNIT, label: 'Unit' },
    { value: RackSlotType.POWER, label: 'Power' },
    { value: RackSlotType.ZERO_U, label: 'Zero-U' },
  ];

  /** Racks grouped by data center, so the place can be picked before the rack. */
  private readonly racksByDatacenter = computed(() => {
    const groups = new Map<string, RackOption[]>();
    this.racks().forEach((rack) => {
      const list = groups.get(rack.datacenter) ?? [];
      list.push(rack);
      groups.set(rack.datacenter, list);
    });
    return [...groups.entries()]
      .map(([datacenter, racks]) => ({ datacenter, racks }))
      .sort((a, b) => a.datacenter.localeCompare(b.datacenter));
  });

  protected readonly locations = computed(() =>
    this.racksByDatacenter().map((group) => group.datacenter),
  );

  /** The place the rack list is limited to. Falls back to the first, so the
   *  rack picker always has something to show. */
  protected readonly rackLocation = computed(
    () => this.pickedLocation() || this.locations()[0] || '',
  );

  protected readonly racksAtLocation = computed(
    () =>
      this.racksByDatacenter().find((group) => group.datacenter === this.rackLocation())?.racks ??
      [],
  );

  private readonly sheetEl =
    viewChild<ElementRef<HTMLElement & { show?: () => void; hide?: () => void }>>('assetSheet');

  private readonly deviceBox = viewChild<{ nativeElement: { value: string } }>('deviceBox');

  private readonly fAssetTag = viewChild<NativeElementRef>('fAssetTag');

  private readonly fAssetSerial = viewChild<NativeElementRef>('fAssetSerial');

  private readonly fAssetWarranty = viewChild<NativeElementRef>('fAssetWarranty');

  private readonly fAssetRackUnit = viewChild<NativeElementRef>('fAssetRackUnit');

  private readonly fAssetNotes = viewChild<NativeElementRef>('fAssetNotes');

  constructor() {
    effect(() => {
      const el = this.sheetEl()?.nativeElement;
      if (this.form() !== null) {
        el?.show?.();
        this.focusMarkedField();
      } else el?.hide?.();
    });
    // The device box reads its label off the menu, and the menu arrives with
    // the catalog. On an existing asset the value is set before that, so it is
    // handed over again once there is something to match. Cleared first: the
    // same value set twice is not a change, and the label follows the change.
    effect(() => {
      const id = this.assetDeviceId();
      const el = this.deviceBox()?.nativeElement;
      if (!id || !el || this.catalog().length === 0) return;
      untracked(() => {
        el.value = '';
        el.value = id;
      });
    });
    effect(() => {
      const asset = this.form();
      if (!asset) return;
      this.invalidFields.set({});
      this.formErrorMessage.set(null);
      this.assetDeviceId.set(asset.deviceCatalogId ?? '');
      this.assetStatus.set(asset.status ?? 'available');
      if (this.catalog().length === 0) this.loadCatalog();
      if (this.racks().length === 0) this.loadRackOptions();
      if (asset.id) this.loadPlacement(asset.id);
      else {
        this.placement.set(null);
        this.assetRackId.set('');
        this.assetSlotType.set('');
        this.pickedLocation.set('');
      }
    });
  }

  /**
   * Focuses the field carrying `autofocus`, for the one open where the mark is
   * not there yet.
   *
   * The sheet reads `[autofocus]` inside `show()`, and which field carries it
   * depends on whether this is a new asset or an existing one — so it is a
   * binding, which Angular writes while refreshing the view, after the effect
   * above has already opened the sheet. Every open after it finds the mark in
   * place, which is why only the first one after a page load landed on nothing.
   */
  private focusMarkedField(): void {
    afterNextRender(
      () => {
        // A task rather than a microtask: the combo box keeps its real input in
        // shadow DOM, which Lit renders on a microtask. By the time this runs an
        // open that focused the field itself has nothing left to do.
        setTimeout(() => {
          const target = this.sheetEl()?.nativeElement.querySelector<HTMLElement>('[autofocus]');
          if (target && document.activeElement !== target) target.focus();
        });
      },
      { injector: this.injector },
    );
  }

  protected isFieldInvalid(field: string): boolean {
    return field in this.invalidFields();
  }

  protected fieldError(field: string): string {
    return this.invalidFields()[field] ?? '';
  }

  /** One status at a time: unpicking the current one leaves it as it was. */
  protected onStatusToggle(status: AssetStatus, selected: boolean): void {
    if (selected) this.assetStatus.set(status);
  }

  protected onSlotTypeToggle(slotType: RackSlotType, selected: boolean): void {
    if (selected) this.assetSlotType.set(slotType);
  }

  /** Picking a place empties the rack: the rack you had stands somewhere else. */
  protected onLocationToggle(location: string, selected: boolean): void {
    if (!selected) return;
    this.pickedLocation.set(location);
    this.assetRackId.set('');
  }

  protected close(): void {
    this.overlays.closeAsset();
  }

  protected save(): void {
    const form = this.form();
    if (!form) return;
    this.invalidFields.set({});
    this.formErrorMessage.set(null);
    const deviceCatalogId = this.assetDeviceId() || (form.deviceCatalogId ?? '');
    // An asset is an instance of a product, so without one there is nothing to
    // create. Said here rather than left to the API, because the answer is in
    // the field right above the button.
    if (!deviceCatalogId) {
      this.invalidFields.set({ device_catalog_id: 'Pick the device this asset is.' });
      return;
    }
    const entry = this.catalogById.get(deviceCatalogId);
    const warranty = this.fAssetWarranty()?.nativeElement.value ?? '';
    const updated: Asset = {
      id: form.id ?? '',
      deviceCatalogId,
      model: entry?.model ?? form.model ?? 'Unknown device',
      category: entry?.category ?? form.category ?? 'Other',
      assetTag: this.fAssetTag()?.nativeElement.value ?? '',
      status: this.assetStatus(),
      serialNumber: this.fAssetSerial()?.nativeElement.value ?? '',
      warrantyExpiry: warranty || undefined,
      notes: this.fAssetNotes()?.nativeElement.value ?? '',
    };
    const request = form.id
      ? firstValueFrom(this.inventoryApi.updateAsset(updated)).then(() => updated.id)
      : firstValueFrom(this.inventoryApi.createAsset(updated)).then((res) => res.assetId);
    request
      .then((assetId) => this.reconcilePlacement(assetId))
      .then(() => {
        this.stats.markChanged();
        this.overlays.closeAsset();
      })
      .catch((err) => {
        // The rack-unit guard rejects with its message already in place.
        if (err instanceof Error && err.message === 'invalid rack unit') return;
        const { fields, message } = parseValidationError(err);
        this.invalidFields.set(fields);
        this.formErrorMessage.set(message);
      });
  }

  /**
   * Writes where the asset stands. A rack without a unit is refused here: the
   * rack only draws units 1…totalU, so a device placed at U0 stands in it
   * without being anywhere you can see.
   */
  private reconcilePlacement(assetId: string): Promise<unknown> {
    const rackId = this.assetRackId();
    const slotType = this.assetSlotType() || RackSlotType.UNIT;
    const existingPlacementId = this.placement()?.id ?? null;
    if (!rackId) {
      // No rack: this clears an existing placement, and the unit is moot.
      return this.placementApi.reconcilePlacement({
        assetId,
        rackId: '',
        unit: 0,
        slotType,
        existingPlacementId,
      });
    }
    const unit = parseInt(this.fAssetRackUnit()?.nativeElement.value ?? '', 10);
    if (!Number.isInteger(unit) || unit < 1) {
      this.invalidFields.set({ rack_unit_start: 'Enter a rack unit of 1 or higher.' });
      return Promise.reject(new Error('invalid rack unit'));
    }
    return this.placementApi.reconcilePlacement({
      assetId,
      rackId,
      unit,
      slotType,
      existingPlacementId,
    });
  }

  /** Where the asset stands, resolved before the pickers are read: the place
   *  comes from the rack it is in, and the rack list follows the place. */
  private loadPlacement(assetId: string): void {
    firstValueFrom(this.placementApi.getPlacementByAsset(assetId))
      .then((res) => {
        const p = res.placement;
        const placement =
          p && p.location.case === 'rack'
            ? {
                id: p.id,
                rackId: p.location.value.rackId,
                unit: p.location.value.rackUnitStart,
                slotType: p.location.value.rackSlotType,
              }
            : null;
        this.placement.set(placement);
        this.assetRackId.set(placement?.rackId ?? '');
        this.assetSlotType.set(placement?.slotType ?? '');
        this.pickedLocation.set(
          this.racks().find((rack) => rack.id === placement?.rackId)?.datacenter ?? '',
        );
      })
      .catch((err) => {
        this.placement.set(null);
        this.assetRackId.set('');
        this.assetSlotType.set('');
        this.pickedLocation.set('');
        // eslint-disable-next-line no-console
        console.error(connectErrorMessage(err));
      });
  }

  private loadCatalog(): void {
    firstValueFrom(this.catalogApi.listCatalog())
      .then((res) => {
        this.catalogById = new Map(
          res.entries
            .filter((s) => s.entry)
            .map((s) => {
              const entry = CatalogApiService.mapCatalogEntry(s.entry!);
              return [entry.id, entry] as const;
            }),
        );
        this.catalog.set([...this.catalogById.values()]);
      })
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));
  }

  private loadRackOptions(): void {
    this.placementApi
      .listRackOptions()
      .then((racks) => this.racks.set(racks))
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));
  }
}
