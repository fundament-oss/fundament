import { inject, Injectable, signal } from '@angular/core';
import { firstValueFrom } from 'rxjs';
import { Cable, Port } from './cable.model';
import PatchMappingApiService from './patch-mapping-api.service';
import DatacenterApiService from '../datacenters/datacenter-api.service';
import PlacementApiService from '../inventory/placement-api.service';
import CatalogApiService from '../catalog/catalog-api.service';
import { PortDefinition } from '../inventory/inventory';
import { ASSET_CLIENT } from '../../connect/tokens';
import connectErrorMessage from '../../connect/error';
import parseValidationError from '../../connect/validation';
import { cablePortFromDefinition, cablePortTypeToDefinition } from '../racks/catalog-helpers';

/** Everything one data center holds, as far as cabling is concerned. */
export interface SiteGraph {
  devices: DeviceOption[];
  devicePorts: Record<string, Port[]>;
  cables: Cable[];
}

const EMPTY_GRAPH: SiteGraph = { devices: [], devicePorts: {}, cables: [] };

/** A selectable device (placement) in a data center. */
export interface DeviceOption {
  id: string;
  name: string;
}

/** Maps a cabling UI port onto the catalog port-definition shape for writes. */
function toPortDefinition(port: Port, catalogId: string, ordinal?: number): PortDefinition {
  const { portType, direction } = cablePortTypeToDefinition(port.type);
  return {
    id: port.id.startsWith('local-') ? '' : port.id,
    catalogEntryId: catalogId,
    name: port.name,
    portType,
    direction,
    ordinal: ordinal ?? 0,
  };
}

/**
 * Everything a data center is made of, as far as cabling is concerned: the
 * devices in it, the ports each of them has, and the cables between them.
 *
 * Held here rather than in the patch mapping page, because the cable form is
 * opened from the bar as well and has to be able to ask for another data center
 * than the one a page happens to be showing.
 */
@Injectable({ providedIn: 'root' })
export default class PatchGraphService {
  private readonly patchApi = inject(PatchMappingApiService);

  private readonly dcApi = inject(DatacenterApiService);

  private readonly placementApi = inject(PlacementApiService);

  private readonly catalogApi = inject(CatalogApiService);

  private readonly assetClient = inject(ASSET_CLIENT);

  /**
   * One graph per data center, not one at a time.
   *
   * The page shows one data center and the cable form may be pointed at
   * another, so a single "current" graph would make picking a data center in
   * the form change the list behind it.
   */
  private readonly bySite = signal<Record<string, SiteGraph>>({});

  /** What went wrong writing ports back, for the form to show. */
  readonly error = signal<string | null>(null);

  private readonly catalogByPlacement = new Map<string, Map<string, string>>();

  /** The graph of one data center; empty until it is loaded. */
  graphFor(siteId: string): SiteGraph {
    return this.bySite()[siteId] ?? EMPTY_GRAPH;
  }

  /**
   * Loads every device (placement) in the site, its ports (from the catalog
   * port definitions), and the physical connections between them.
   */
  async load(siteId: string): Promise<void> {
    try {
      const racksRes = await firstValueFrom(this.dcApi.listRacksBySite(siteId));
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

      this.catalogByPlacement.set(siteId, catalogByPlacement);
      this.bySite.update((all) => ({ ...all, [siteId]: { devices, devicePorts, cables } }));
    } catch (err) {
      // eslint-disable-next-line no-console
      console.error(connectErrorMessage(err));
    }
  }

  /**
   * Persists port edits made from the cable form. Ports are catalog port
   * definitions, so a change to one device also affects every device sharing
   * its catalog entry; the site graph is reloaded afterwards to reconcile
   * synthetic ids with the server-assigned ones.
   */
  /** Writes port edits back to the catalog, then reads the site again. */
  async applyPortsUpdate(
    siteId: string,
    event: { deviceId: string; ports: Port[] },
  ): Promise<void> {
    const catalogId = this.catalogByPlacement.get(siteId)?.get(event.deviceId);
    if (!catalogId) return;

    const oldPorts = this.graphFor(siteId).devicePorts[event.deviceId] ?? [];
    const newPorts = event.ports;
    const oldById = new Map(oldPorts.map((p) => [p.id, p]));
    const newById = new Map(newPorts.map((p) => [p.id, p]));

    const deletions = oldPorts.filter((p) => !newById.has(p.id));
    const creations = newPorts.filter((p) => !oldById.has(p.id));
    const updates = newPorts.filter((p) => {
      const prev = oldById.get(p.id);
      return prev != null && (prev.name !== p.name || prev.type !== p.type);
    });

    if (deletions.length === 0 && creations.length === 0 && updates.length === 0) return;

    // Optimistic update so the form reflects the change before the round-trip.
    this.bySite.update((all) => ({
      ...all,
      [siteId]: {
        ...this.graphFor(siteId),
        devicePorts: { ...this.graphFor(siteId).devicePorts, [event.deviceId]: newPorts },
      },
    }));

    try {
      await Promise.all([
        ...deletions.map((p) => firstValueFrom(this.catalogApi.deletePortDefinition(p.id))),
        ...updates.map((p) =>
          firstValueFrom(this.catalogApi.updatePortDefinition(toPortDefinition(p, catalogId))),
        ),
      ]);
      // New definitions take ordinals after the existing ports.
      const baseOrdinal = oldPorts.length;
      await Promise.all(
        creations.map((p, i) =>
          firstValueFrom(
            this.catalogApi.createPortDefinition(toPortDefinition(p, catalogId, baseOrdinal + i)),
          ),
        ),
      );
      this.error.set(null);
    } catch (err) {
      const { fields, message } = parseValidationError(err);
      const all = [message, ...Object.values(fields)].filter(Boolean);
      this.error.set(all.join('\n') || 'Failed to save port changes.');
    } finally {
      await this.load(siteId);
    }
  }
}
