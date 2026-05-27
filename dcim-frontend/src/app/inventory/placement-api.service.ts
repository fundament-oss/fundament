import { Injectable, inject } from '@angular/core';
import { firstValueFrom } from 'rxjs';
import { RackSlotType } from '../../generated/v1/common_pb';
import {
  PLACEMENT_CLIENT,
  RACK_CLIENT,
  RACK_ROW_CLIENT,
  ROOM_CLIENT,
  SITE_CLIENT,
} from '../../connect/tokens';

/** A rack the user can place an asset in, with its datacenter for grouping. */
export interface RackOption {
  id: string;
  name: string;
  datacenter: string;
}

@Injectable({ providedIn: 'root' })
export default class PlacementApiService {
  private readonly placementClient = inject(PLACEMENT_CLIENT);

  private readonly rackClient = inject(RACK_CLIENT);

  private readonly rackRowClient = inject(RACK_ROW_CLIENT);

  private readonly roomClient = inject(ROOM_CLIENT);

  private readonly siteClient = inject(SITE_CLIENT);

  /**
   * Loads every rack and resolves its datacenter by walking
   * rack -> rack row -> room -> site, so racks can be grouped for selection.
   */
  async listRackOptions(): Promise<RackOption[]> {
    const [racksRes, rowsRes, roomsRes, sitesRes] = await Promise.all([
      firstValueFrom(this.rackClient.listRacks({})),
      firstValueFrom(this.rackRowClient.listRackRows({})),
      firstValueFrom(this.roomClient.listRooms({})),
      firstValueFrom(this.siteClient.listSites({})),
    ]);

    const siteName = new Map(sitesRes.sites.map((s) => [s.id, s.name]));
    const roomSite = new Map(roomsRes.rooms.map((r) => [r.id, r.siteId]));
    const rowRoom = new Map(rowsRes.rackRows.map((r) => [r.id, r.roomId]));

    return racksRes.racks
      .map((summary) => summary.rack)
      .filter((rack): rack is NonNullable<typeof rack> => rack != null)
      .map((rack) => {
        const roomId = rowRoom.get(rack.rowId);
        const siteId = roomId ? roomSite.get(roomId) : undefined;
        return {
          id: rack.id,
          name: rack.name,
          datacenter: (siteId && siteName.get(siteId)) || 'Unknown',
        };
      });
  }

  getPlacementByAsset(assetId: string) {
    return this.placementClient.getPlacementByAsset({ assetId });
  }

  createPlacement(assetId: string, rackId: string, rackUnitStart: number, slotType: RackSlotType) {
    return this.placementClient.createPlacement({
      assetId,
      location: { case: 'rack', value: { rackId, rackUnitStart, rackSlotType: slotType } },
    });
  }

  updatePlacement(
    placementId: string,
    rackId: string,
    rackUnitStart: number,
    slotType: RackSlotType,
  ) {
    return this.placementClient.updatePlacement({
      id: placementId,
      location: { case: 'rack', value: { rackId, rackUnitStart, rackSlotType: slotType } },
    });
  }

  deletePlacement(id: string) {
    return this.placementClient.deletePlacement({ id });
  }

  /**
   * Creates, updates, or removes the asset's placement so it matches the
   * given rack/unit/slot. Pass `existingPlacementId` when the asset already
   * has a placement; pass an empty `rackId` to clear the placement.
   */
  reconcilePlacement(input: {
    assetId: string;
    rackId: string;
    unit: number;
    slotType: RackSlotType;
    existingPlacementId: string | null;
  }): Promise<unknown> {
    const { assetId, rackId, unit, slotType, existingPlacementId } = input;
    if (!rackId) {
      return existingPlacementId
        ? firstValueFrom(this.deletePlacement(existingPlacementId))
        : Promise.resolve();
    }
    return existingPlacementId
      ? firstValueFrom(this.updatePlacement(existingPlacementId, rackId, unit, slotType))
      : firstValueFrom(this.createPlacement(assetId, rackId, unit, slotType));
  }
}
