import { Injectable, inject } from '@angular/core';
import type { PhysicalConnection as ProtoConn } from '../../generated/v1/connection_pb';
import type { PhysicalConnection } from './patch-mapping.model';
import { PHYSICAL_CONNECTION_CLIENT } from '../../connect/tokens';

@Injectable({ providedIn: 'root' })
export default class PatchMappingApiService {
  private readonly client = inject(PHYSICAL_CONNECTION_CLIENT);

  static mapConnection(c: ProtoConn): PhysicalConnection {
    return {
      id: c.id,
      dcId: '',
      sourcePlacementId: c.sourcePlacementId,
      sourceDeviceLabel: c.sourcePlacementId,
      sourcePortName: c.sourcePortName,
      targetPlacementId: c.targetPlacementId,
      targetDeviceLabel: c.targetPlacementId,
      targetPortName: c.targetPortName,
      cableAssetId: c.cableAssetId || undefined,
    };
  }

  listConnectionsByPlacement(placementId: string) {
    return this.client.listConnectionsByPlacement({ placementId });
  }

  createPhysicalConnection(
    sourcePlacementId: string,
    sourcePortName: string,
    targetPlacementId: string,
    targetPortName: string,
  ) {
    return this.client.createPhysicalConnection({
      sourcePlacementId,
      sourcePortName,
      targetPlacementId,
      targetPortName,
    });
  }

  updatePhysicalConnection(id: string, notes: string) {
    return this.client.updatePhysicalConnection({ id, notes });
  }

  deletePhysicalConnection(id: string) {
    return this.client.deletePhysicalConnection({ id });
  }
}
