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
import DatacenterApiService from '../datacenter-api.service';
import DatacenterListService from '../datacenter-list.service';
import { DatacenterInfo, DatacenterStatus } from '../datacenter.model';
import OverlayService from '../../shell/overlay.service';
import parseValidationError from '../../../connect/validation';

interface NativeElementRef {
  nativeElement: { value: string; show?: () => void; hide?: () => void };
}

/**
 * The form for one data center, held by the shell rather than by a page.
 *
 * A data center is made from anywhere: from the add button in the bar, from the
 * list, from a data center you already have open. A page that unmounts on
 * navigation cannot hold a sheet that has to outlive it, so this lives beside
 * the pages and reads which record it is on from OverlayService.
 *
 * It writes into DatacenterListService, which every page reads, so a new or
 * renamed data center shows up wherever it is listed without a refetch.
 */
@Component({
  selector: 'app-datacenter-sheet',
  changeDetection: ChangeDetectionStrategy.OnPush,
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  templateUrl: './datacenter-sheet.html',
})
export default class DatacenterSheetComponent {
  private readonly dcApi = inject(DatacenterApiService);

  private readonly list = inject(DatacenterListService);

  protected readonly overlays = inject(OverlayService);

  protected readonly form = this.overlays.datacenterSheet;

  protected readonly isEdit = computed(() => !!this.form()?.id);

  /** The four tiers, as buttons rather than a dropdown. */
  protected readonly TIERS: { value: string; label: string }[] = [
    { value: '1', label: 'Tier 1' },
    { value: '2', label: 'Tier 2' },
    { value: '3', label: 'Tier 3' },
    { value: '4', label: 'Tier 4' },
  ];

  protected readonly DC_STATUSES: { value: DatacenterStatus; label: string }[] = [
    { value: 'operational', label: 'Operational' },
    { value: 'degraded', label: 'Degraded' },
    { value: 'maintenance', label: 'Maintenance' },
  ];

  protected readonly dcTier = signal('3');

  protected readonly dcStatus = signal<DatacenterStatus>('operational');

  protected readonly invalidFields = signal<Record<string, string>>({});

  protected readonly formErrorMessage = signal<string | null>(null);

  private readonly sheetEl = viewChild<NativeElementRef>('dcSheet');

  private readonly fName = viewChild<NativeElementRef>('fName');

  private readonly fFullName = viewChild<NativeElementRef>('fFullName');

  private readonly fCity = viewChild<NativeElementRef>('fCity');

  private readonly fCountry = viewChild<NativeElementRef>('fCountry');

  private readonly fAddress = viewChild<NativeElementRef>('fAddress');

  private readonly fEstablished = viewChild<NativeElementRef>('fEstablished');

  private readonly fFloorSqm = viewChild<NativeElementRef>('fFloorSqm');

  constructor() {
    effect(() => {
      const el = this.sheetEl()?.nativeElement;
      if (this.form() !== null) el?.show?.();
      else el?.hide?.();
    });
    // The two toggle groups keep their own state, so they follow the record the
    // sheet was opened on.
    effect(() => {
      const dc = this.form();
      if (!dc) return;
      this.dcTier.set(String(dc.tier ?? 3));
      this.dcStatus.set(dc.status ?? 'operational');
      this.invalidFields.set({});
      this.formErrorMessage.set(null);
    });
  }

  protected isFieldInvalid(field: string): boolean {
    return !!this.invalidFields()[field];
  }

  protected fieldError(field: string): string {
    return this.invalidFields()[field] ?? '';
  }

  /** One tier at a time: unpicking the current one leaves it as it was. */
  protected onTierToggle(tier: string, selected: boolean): void {
    if (selected) this.dcTier.set(tier);
  }

  protected onDcStatusToggle(status: DatacenterStatus, selected: boolean): void {
    if (selected) this.dcStatus.set(status);
  }

  protected close(): void {
    this.overlays.closeDatacenter();
  }

  protected save(): void {
    const form = this.form();
    if (!form) return;
    this.invalidFields.set({});
    this.formErrorMessage.set(null);
    const updated: DatacenterInfo = {
      id: form.id || `dc-${Date.now()}`,
      name: this.fName()?.nativeElement.value ?? '',
      fullName: this.fFullName()?.nativeElement.value ?? '',
      city: this.fCity()?.nativeElement.value ?? '',
      country: this.fCountry()?.nativeElement.value ?? '',
      address: this.fAddress()?.nativeElement.value ?? '',
      tier: (parseInt(this.dcTier(), 10) || 3) as 1 | 2 | 3 | 4,
      status: this.dcStatus(),
      established: parseFloat(this.fEstablished()?.nativeElement.value ?? '0') || 0,
      floorSqm: parseFloat(this.fFloorSqm()?.nativeElement.value ?? '0') || 0,
      // Not modelled by the API.
      powerCapacityKw: 0,
      coolingCapacityKw: 0,
      pue: 0,
    };
    const request = form.id
      ? firstValueFrom(this.dcApi.updateSite(updated)).then(() => updated)
      : firstValueFrom(this.dcApi.createSite(updated)).then((res) => ({
          ...updated,
          id: res.siteId || updated.id,
        }));
    request
      .then((saved) => {
        this.list.datacenters.update((all) =>
          form.id ? all.map((dc) => (dc.id === form.id ? saved : dc)) : [...all, saved],
        );
        this.overlays.closeDatacenter();
      })
      .catch((err) => {
        const { fields, message } = parseValidationError(err);
        this.invalidFields.set(fields);
        this.formErrorMessage.set(message);
      });
  }
}
