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
import CableFormComponent from '../cable-form/cable-form';
import { Cable, Port } from '../cable.model';
import PatchGraphService from '../patch-graph.service';
import PatchMappingApiService from '../patch-mapping-api.service';
import DatacenterListService from '../../datacenters/datacenter-list.service';
import OverlayService from '../../shell/overlay.service';
import parseValidationError from '../../../connect/validation';

interface NativeElementRef {
  nativeElement: { show?: () => void; hide?: () => void };
}

/**
 * The form for one cable, held by the shell rather than by the patch mapping
 * page.
 *
 * Which data center the cable runs in is a question the form asks, not one the
 * page answers: opened from the bar there is no page to inherit it from. It
 * starts on the one you were looking at, and otherwise on the first.
 */
@Component({
  selector: 'app-cable-sheet',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [CableFormComponent],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  templateUrl: './cable-sheet.html',
})
export default class CableSheetComponent {
  private readonly patchApi = inject(PatchMappingApiService);

  private readonly graph = inject(PatchGraphService);

  private readonly datacenterList = inject(DatacenterListService);

  protected readonly overlays = inject(OverlayService);

  protected readonly form = this.overlays.cableSheet;

  protected readonly datacenters = this.datacenterList.datacenters;

  /** The data center this cable runs in. */
  protected readonly dcId = signal('');

  private readonly siteGraph = computed(() => this.graph.graphFor(this.dcId()));

  protected readonly devices = computed(() => this.siteGraph().devices);

  protected readonly devicePorts = computed(() => this.siteGraph().devicePorts);

  protected readonly cables = computed(() => this.siteGraph().cables);

  protected readonly formError = signal<string | null>(null);

  private readonly sheetEl = viewChild<NativeElementRef>('cableSheet');

  constructor() {
    effect(() => {
      const el = this.sheetEl()?.nativeElement;
      if (this.form() !== null) el?.show?.();
      else el?.hide?.();
    });
    effect(() => {
      const cable = this.form();
      if (!cable) return;
      this.formError.set(null);
      if (this.datacenters().length === 0) this.datacenterList.load();
      const dcId = cable.dcId || this.dcId() || this.datacenters()[0]?.id || '';
      this.dcId.set(dcId);
      if (dcId && this.devices().length === 0) this.graph.load(dcId).catch(() => undefined);
    });
  }

  /** Another data center means another set of devices, so the form empties its
   *  two ends and this loads what stands in the new one. */
  protected onDcChange(id: string): void {
    if (!id || id === this.dcId()) return;
    this.dcId.set(id);
    this.graph.load(id).catch(() => undefined);
  }

  protected onPortsUpdated(event: { deviceId: string; ports: Port[] }): void {
    this.graph.applyPortsUpdate(this.dcId(), event).catch(() => undefined);
  }

  protected close(): void {
    this.overlays.closeCable();
  }

  protected save(cable: Cable): void {
    this.formError.set(null);
    const request = cable.id
      ? firstValueFrom(this.patchApi.updateCable(cable))
      : firstValueFrom(this.patchApi.createCable(cable));
    request
      .then(() => {
        this.overlays.closeCable();
        return this.graph.load(this.dcId());
      })
      .catch((err) => {
        const { fields, message } = parseValidationError(err);
        const all = [message, ...Object.values(fields)].filter(Boolean);
        this.formError.set(all.join('\n') || 'Failed to save cable.');
      });
  }
}
