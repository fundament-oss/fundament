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
import CableFormComponent from './cable-form/cable-form';
import ShoppingListComponent from './shopping-list/shopping-list';
import { Cable, CABLE_TYPE_LABEL, CableSide, CableStatus, CableType, Port } from './cable.model';
import PatchMappingApiService from './patch-mapping-api.service';
import DatacenterApiService from '../datacenters/datacenter-api.service';
import PlacementApiService from '../inventory/placement-api.service';
import CatalogApiService from '../catalog/catalog-api.service';
import { ASSET_CLIENT } from '../../connect/tokens';
import connectErrorMessage from '../../connect/error';
import parseValidationError from '../../connect/validation';
import { cablePortFromDefinition } from '../racks/catalog-helpers';

/** A selectable device (placement) in the active datacenter. */
interface DeviceOption {
  id: string;
  name: string;
}

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
    CableFormComponent,
    ShoppingListComponent,
  ],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  host: {
    class: 'flex flex-col bg-white dark:bg-gray-950 text-slate-900 dark:text-white',
    '[class.min-h-screen]': "activeView() === 'list'",
    '[class.overflow-hidden]': "activeView() === 'topology'",
    '[style.height]': "activeView() === 'topology' ? 'calc(100dvh - 4.25rem)' : null",
  },
  templateUrl: './patch-mapping.html',
})
export default class PatchMappingComponent implements OnInit {
  private readonly patchApi = inject(PatchMappingApiService);

  private readonly datacenterApi = inject(DatacenterApiService);

  private readonly placementApi = inject(PlacementApiService);

  private readonly catalogApi = inject(CatalogApiService);

  private readonly assetClient = inject(ASSET_CLIENT);

  readonly sites = signal<SiteOption[]>([]);

  readonly selectedDcId = signal('');

  readonly activeView = signal<'list' | 'topology'>('list');

  // ── Cable state (cables of the selected datacenter) ─────────────────────────
  readonly mutableCables = signal<Cable[]>([]);

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

  readonly topologyTypeFilter = signal<CableType | ''>('');

  // Devices (placements) and their ports in the active datacenter.
  readonly dcDevices = signal<DeviceOption[]>([]);

  readonly localDevicePorts = signal<Record<string, Port[]>>({});

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

  constructor() {
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
    // loadSiteGraph handles its own errors; the no-op catch just marks the
    // promise as handled for the floating-promise lint.
    this.loadSiteGraph(siteId).catch(() => undefined);
  }

  /**
   * Loads every device (placement) in the site, its ports (from the catalog
   * port definitions), and the physical connections between them.
   */
  private async loadSiteGraph(siteId: string): Promise<void> {
    try {
      const racksRes = await firstValueFrom(this.datacenterApi.listRacksBySite(siteId));
      const rackIds = racksRes.racks
        .map((s) => s.rack?.id)
        .filter((id): id is string => id != null);

      const [placementArrays, assetsRes] = await Promise.all([
        Promise.all(
          rackIds.map((id) => firstValueFrom(this.placementApi.listPlacementsByRack(id))),
        ),
        firstValueFrom(this.assetClient.listAssets({})),
      ]);
      const placements = placementArrays
        .flatMap((r) => r.placements)
        .filter((p) => p.location.case === 'rack');
      const assetById = new Map(assetsRes.assets.map((a) => [a.id, a]));

      const devices: DeviceOption[] = [];
      const catalogByPlacement = new Map<string, string>();
      placements.forEach((p) => {
        const asset = assetById.get(p.assetId);
        devices.push({ id: p.id, name: asset?.assetTag || p.assetId });
        if (asset?.deviceCatalogId) catalogByPlacement.set(p.id, asset.deviceCatalogId);
      });
      devices.sort((a, b) => a.name.localeCompare(b.name));

      // Port definitions per unique catalog entry.
      const uniqueCatalogIds = [...new Set(catalogByPlacement.values())];
      const portDefArrays = await Promise.all(
        uniqueCatalogIds.map((id) => firstValueFrom(this.catalogApi.listPortDefinitions(id))),
      );
      const portDefsByCatalog = new Map(
        uniqueCatalogIds.map((id, i) => [id, portDefArrays[i].portDefinitions]),
      );

      const devicePorts: Record<string, Port[]> = {};
      const portById = new Map<string, Port>();
      placements.forEach((p) => {
        const catalogId = catalogByPlacement.get(p.id);
        const defs = catalogId ? (portDefsByCatalog.get(catalogId) ?? []) : [];
        const ports = defs
          .map((pd) => cablePortFromDefinition(pd, p.id))
          .filter((port): port is Port => port !== null);
        devicePorts[p.id] = ports;
        ports.forEach((port) => portById.set(port.id, port));
      });

      // Every connection in the site, fetched in a single call.
      const connRes = await firstValueFrom(this.patchApi.listConnectionsBySite(siteId));
      const deviceNameById = new Map(devices.map((d) => [d.id, d.name]));
      const cables = connRes.connections.map((c) =>
        PatchMappingApiService.mapConnection(c, siteId, { deviceNameById, portById }),
      );

      this.dcDevices.set(devices);
      this.localDevicePorts.set(devicePorts);
      this.mutableCables.set(cables);
    } catch (err) {
      // eslint-disable-next-line no-console
      console.error(connectErrorMessage(err));
    }
  }

  // ── CRUD actions ───────────────────────────────────────────────────────────

  openAddCable(): void {
    this.cableFormError.set(null);
    this.editCable.set({ dcId: this.selectedDcId(), status: 'connected' });
  }

  openEditCable(cable: Cable): void {
    this.cableFormError.set(null);
    this.editCable.set({ ...cable });
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

  saveFromForm(cable: Cable): void {
    this.cableFormError.set(null);
    const request = cable.id
      ? firstValueFrom(this.patchApi.updateCable(cable))
      : firstValueFrom(this.patchApi.createCable(cable));
    request
      .then(() => {
        this.editCable.set(null);
        return this.loadSiteGraph(this.selectedDcId());
      })
      .catch((err) => {
        const { fields, message } = parseValidationError(err);
        const all = [message, ...Object.values(fields)].filter(Boolean);
        this.cableFormError.set(all.join('\n') || 'Failed to save cable.');
      });
  }

  closeForm(): void {
    this.cableFormError.set(null);
    this.editCable.set(null);
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
        return this.loadSiteGraph(this.selectedDcId());
      })
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));
  }

  updateCableStatus(event: { cableId: string; status: CableStatus }): void {
    const cable = this.mutableCables().find((c) => c.id === event.cableId);
    if (!cable) return;
    firstValueFrom(this.patchApi.updateCable({ ...cable, status: event.status }))
      .then(() => this.loadSiteGraph(this.selectedDcId()))
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));
  }
}
