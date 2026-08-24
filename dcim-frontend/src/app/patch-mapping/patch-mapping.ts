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
import {
  Cable,
  CABLE_STATUSES,
  CABLE_TYPE_LABEL,
  CableSide,
  CableStatus,
  CableType,
} from './cable.model';
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
export interface SiteOption {
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

  // ── Cable state ────────────────────────────────────────────────────────────
  /**
   * The list spans every location unless you narrow it to one.
   *
   * A cable is a thing in a building, but the questions you ask a list of them
   * are not: what still has to be bought, what is waiting to be fitted. Those
   * are estate-wide, and the badge on this section in the menu counts them that
   * way. Opening in one building would have that badge and this page disagree
   * from the first second.
   */
  private readonly siteGraph = computed(() =>
    this.selectedDcId() ? this.graph.graphFor(this.selectedDcId()) : null,
  );

  readonly mutableCables = computed(
    () => this.siteGraph()?.cables ?? this.graph.allCables(),
  );

  readonly dcCables = computed(() => this.mutableCables());

  readonly editCable = signal<Partial<Cable> | null>(null);

  /** Server-side error from the last cable save, shown in the cable form banner. */
  readonly cableFormError = signal<string | null>(null);

  readonly deleteCable = signal<Cable | null>(null);

  // ── Shopping list state ────────────────────────────────────────────────────
  readonly shoppingListOpen = signal(false);

  /**
   * Every planned cable, wherever it lies.
   *
   * The shopping list is one order for the estate, not per building: you can
   * switch between all of them and one location inside the sheet, so it needs
   * them all. The views on this page stay per data center.
   */
  readonly orderCables = computed(() =>
    this.graph.allCables().filter((c) => c.status === 'to-order'),
  );

  private readonly shoppingList = viewChild(ShoppingListComponent);

  /**
   * Opens the list on where you were standing, so it continues the view behind
   * it rather than jumping. You widen it to the whole estate with the toggle in
   * the sheet, because an order is not a building.
   */
  openShoppingList(): void {
    this.shoppingList()?.openAt(this.selectedDcId());
    this.shoppingListOpen.set(true);
  }

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
  /**
   * The drawing is of one hall, so it has a location of its own.
   *
   * Two data centers on one canvas is not a drawing but two islands side by
   * side, so All is a list idea and the topology picks a building itself. Same
   * control, in the same place, as the shopping list.
   */
  readonly topologyDcId = signal('');

  private readonly topologyGraph = computed(() => this.graph.graphFor(this.topologyDcId()));

  readonly topologyCables = computed(() => this.topologyGraph().cables);

  readonly dcDevices = computed(() => this.topologyGraph().devices);

  readonly localDevicePorts = computed(() => this.topologyGraph().devicePorts);

  /** Opens on the building you were looking at, or on the first one when the
   *  list is showing all of them. */
  openTopology(): void {
    if (!this.topologyDcId()) {
      this.topologyDcId.set(this.selectedDcId() || this.sites()[0]?.id || '');
    }
    this.topologyOpen.set(true);
  }

  onTopologyDcToggle(id: string, selected: boolean): void {
    if (selected) {
      this.topologyDcId.set(id);
      this.graph.load(id).catch(() => undefined);
    }
  }

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

  readonly CABLE_STATUSES = CABLE_STATUSES;

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
        // No data center is picked: the list opens on all of them, so there is
        // nothing to choose before you have said you want one building.
        this.loadEverySite();
      })
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));
  }

  /** Reads every site once, so the shopping list can count across them before
   *  you have visited each one. */
  private loadEverySite(): void {
    this.sites().forEach((site) => {
      this.graph.load(site.id).catch(() => undefined);
    });
  }

  /** An empty id is every location, which is where the list starts. */
  selectDc(siteId: string): void {
    this.selectedDcId.set(siteId);
    if (!siteId) return;
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

  updateCableStatus(event: { cableId: string; status: CableStatus | undefined }): void {
    this.updateCableStatuses({ cableIds: [event.cableId], status: event.status });
  }

  /**
   * Moves a whole order line at once: several cables, possibly in more than one
   * building, in one go.
   *
   * Looked up across every loaded site rather than in the one on screen,
   * because the shopping list spans them all and a tick there is about cables
   * you are not looking at. Only the sites it touched are read back.
   */
  updateCableStatuses(event: { cableIds: string[]; status: CableStatus | undefined }): void {
    const byId = new Map(this.graph.allCables().map((cable) => [cable.id, cable]));
    const targets = event.cableIds
      .map((id) => byId.get(id))
      .filter((cable): cable is Cable => !!cable);
    if (targets.length === 0) return;

    Promise.all(
      targets.map((cable) =>
        firstValueFrom(this.patchApi.updateCable({ ...cable, status: event.status })),
      ),
    )
      .then(() =>
        Promise.all([...new Set(targets.map((c) => c.dcId))].map((id) => this.graph.load(id))),
      )
      .then(() => undefined)
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));
  }
}
