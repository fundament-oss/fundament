import {
  ChangeDetectionStrategy,
  Component,
  CUSTOM_ELEMENTS_SCHEMA,
  effect,
  ElementRef,
  inject,
  OnInit,
  signal,
  computed,
  viewChild,
} from '@angular/core';
import { firstValueFrom } from 'rxjs';
import PatchMappingFlowWrapperComponent from './patch-mapping-flow-wrapper';
import CableListComponent from './cable-list/cable-list';
import ShoppingListComponent from './shopping-list/shopping-list';
import { Cable, CABLE_TYPE_LABEL, CableSide, CableStatus, CableType } from './cable.model';
import PatchMappingApiService from './patch-mapping-api.service';
import DatacenterApiService from '../datacenters/datacenter-api.service';
import PlacementApiService from '../inventory/placement-api.service';
import CatalogApiService from '../catalog/catalog-api.service';
import { ASSET_CLIENT } from '../../connect/tokens';
import connectErrorMessage from '../../connect/error';
import DropdownSyncDirective from '../shared/dropdown-sync.directive';
import openOnCreateRequest from '../shell/create-request';
import PatchGraphService from './patch-graph.service';
import OverlayService from '../shell/overlay.service';

/** A selectable device (placement) in the active datacenter. */
/** A selectable datacenter (site). */
interface SiteOption {
  id: string;
  name: string;
}

@Component({
  selector: 'app-patch-mapping',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [
    PatchMappingFlowWrapperComponent,
    CableListComponent,
    ShoppingListComponent,
    DropdownSyncDirective,
  ],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  templateUrl: './patch-mapping.html',
})
export default class PatchMappingComponent implements OnInit {
  private readonly patchApi = inject(PatchMappingApiService);

  /** The devices, ports and cables of one data center, shared with the cable
   *  form the shell holds. */
  private readonly graph = inject(PatchGraphService);

  /** The cable form lives in the shell, so it opens from anywhere. */
  private readonly overlays = inject(OverlayService);

  private readonly datacenterApi = inject(DatacenterApiService);

  private readonly placementApi = inject(PlacementApiService);

  private readonly catalogApi = inject(CatalogApiService);

  private readonly assetClient = inject(ASSET_CLIENT);

  readonly sites = signal<SiteOption[]>([]);

  readonly selectedDcId = signal('');

  readonly topologyOpen = signal(false);

  // ── Cable state (cables of the selected datacenter) ─────────────────────────
  /** What the data center on screen holds, read from the shared graph. */
  private readonly siteGraph = computed(() => this.graph.graphFor(this.selectedDcId()));

  readonly mutableCables = computed(() => this.siteGraph().cables);

  readonly dcCables = computed(() => this.mutableCables());

  readonly editCable = signal<Partial<Cable> | null>(null);

  /** Server-side error from the last cable save, shown in the cable form banner. */
  readonly cableFormError = signal<string | null>(null);

  readonly deleteCable = signal<Cable | null>(null);

  // ── Shopping list state ────────────────────────────────────────────────────
  readonly shoppingListOpen = signal(false);

  readonly plannedCables = computed(() => this.dcCables().filter((c) => c.status === 'planned'));

  readonly plannedCount = computed(() => this.plannedCables().length);

  readonly selectedDcLabel = computed(
    () => this.sites().find((s) => s.id === this.selectedDcId())?.name ?? this.selectedDcId(),
  );

  // ── Topology filters ───────────────────────────────────────────────────────
  readonly topologyStatusFilter = signal<CableStatus | ''>('');

  /** The status the drawing is filtered on: a radio group, so only the button
   *  that becomes selected has anything to say. */
  onTopologyStatusToggle(value: string, selected: boolean): void {
    if (selected) this.topologyStatusFilter.set(value as CableStatus | '');
  }

  readonly topologyTypeFilter = signal<CableType | ''>('');

  // Devices (placements) and their ports in the active datacenter.
  readonly dcDevices = computed(() => this.siteGraph().devices);

  readonly localDevicePorts = computed(() => this.siteGraph().devicePorts);

  /** Catalog entry id per placement (device); ports are catalog port definitions. */
  readonly CABLE_TYPE_LABEL = CABLE_TYPE_LABEL;

  readonly CABLE_TYPES: CableType[] = [
    'cat5e',
    'cat6',
    'cat6a',
    'cat7',
    'cat8',
    'dac',
    'aoc',
    'mmf',
    'smf',
    'power',
    'console',
    'usb',
    'other',
  ];

  readonly CABLE_STATUSES: { value: CableStatus; label: string }[] = [
    { value: 'planned', label: 'Planned' },
    { value: 'connected', label: 'Connected' },
    { value: 'decommissioned', label: 'Decommissioned' },
  ];

  private readonly cableSheetEl = viewChild<ElementRef>('cableSheet');

  private readonly deleteModalEl = viewChild<ElementRef>('deleteModal');

  private readonly shoppingSheetEl = viewChild<ElementRef>('shoppingSheet');

  private readonly topologySheetEl = viewChild<ElementRef>('topologySheet');

  constructor() {
    // The add menu in the bar asks for this form through the address.
    openOnCreateRequest(
      () => this.openAddCable(),
      () => !!this.selectedDcId(),
    );
    effect(() => {
      const el = this.cableSheetEl()?.nativeElement as { show?: () => void; hide?: () => void };
      if (this.editCable() !== null) el?.show?.();
      else el?.hide?.();
    });
    effect(() => {
      const el = this.deleteModalEl()?.nativeElement as { show?: () => void; hide?: () => void };
      if (this.deleteCable() !== null) el?.show?.();
      else el?.hide?.();
    });
    effect(() => {
      const el = this.shoppingSheetEl()?.nativeElement as { show?: () => void; hide?: () => void };
      if (this.shoppingListOpen()) el?.show?.();
      else el?.hide?.();
    });
    effect(() => {
      const el = this.topologySheetEl()?.nativeElement as { show?: () => void; hide?: () => void };
      if (this.topologyOpen()) el?.show?.();
      else el?.hide?.();
    });
  }

  ngOnInit(): void {
    firstValueFrom(this.datacenterApi.listSites())
      .then((res) => {
        const sites = res.sites.map((s) => ({ id: s.id, name: s.name }));
        this.sites.set(sites);
        if (sites.length > 0 && !this.selectedDcId()) {
          this.selectDc(sites[0].id);
        }
      })
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));
  }

  selectDc(siteId: string): void {
    this.selectedDcId.set(siteId);
    // The service handles its own errors; the no-op catch just marks the
    // promise as handled for the floating-promise lint.
    this.graph.load(siteId).catch(() => undefined);
  }

  // ── CRUD actions ───────────────────────────────────────────────────────────

  /** Both routes into the form open the one the shell holds: it outlives this
   *  page, and the add button in the bar opens the same one. The data center on
   *  screen comes along as the form's first answer. */
  openAddCable(): void {
    this.overlays.newCable(this.selectedDcId());
  }

  openEditCable(cable: Cable): void {
    this.overlays.editCable(cable);
  }

  openEditCableById(id: string): void {
    const cable = this.mutableCables().find((c) => c.id === id);
    if (cable) this.openEditCable(cable);
  }

  openAddCableFromConnection(conn: {
    sourceDeviceId: string;
    sourcePortId: string;
    targetDeviceId: string;
    targetPortId: string;
  }): void {
    const devices = this.dcDevices();
    const ports = this.localDevicePorts();
    const aDevice = devices.find((d) => d.id === conn.sourceDeviceId);
    const bDevice = devices.find((d) => d.id === conn.targetDeviceId);
    const aPort = (ports[conn.sourceDeviceId] ?? []).find((p) => p.id === conn.sourcePortId);
    const bPort = (ports[conn.targetDeviceId] ?? []).find((p) => p.id === conn.targetPortId);

    if (!aDevice || !bDevice || !aPort || !bPort) {
      this.openAddCable();
      return;
    }

    const aSide: CableSide = {
      deviceId: conn.sourceDeviceId,
      deviceName: aDevice.name,
      portId: conn.sourcePortId,
      portName: aPort.name,
      portType: aPort.type,
    };
    const bSide: CableSide = {
      deviceId: conn.targetDeviceId,
      deviceName: bDevice.name,
      portId: conn.targetPortId,
      portName: bPort.name,
      portType: bPort.type,
    };

    this.cableFormError.set(null);
    this.editCable.set({ dcId: this.selectedDcId(), status: 'connected', aSide, bSide });
  }

  openDeleteCable(cable: Cable): void {
    this.deleteCable.set(cable);
    this.editCable.set(null);
  }

  cancelDelete(): void {
    this.deleteCable.set(null);
  }

  confirmDelete(): void {
    const target = this.deleteCable();
    if (!target) return;
    firstValueFrom(this.patchApi.deletePhysicalConnection(target.id))
      .then(() => {
        this.deleteCable.set(null);
        return this.graph.load(this.selectedDcId());
      })
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));
  }

  updateCableStatus(event: { cableId: string; status: CableStatus }): void {
    const cable = this.mutableCables().find((c) => c.id === event.cableId);
    if (!cable) return;
    firstValueFrom(this.patchApi.updateCable({ ...cable, status: event.status }))
      .then(() => this.graph.load(this.selectedDcId()))
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));
  }
}
