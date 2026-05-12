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
      sourcePortDefinitionId: c.sourcePortDefinitionId,
      targetPlacementId: c.targetPlacementId,
      targetDeviceLabel: c.targetPlacementId,
      targetPortDefinitionId: c.targetPortDefinitionId,
      cableAssetId: c.cableAssetId || undefined,
    };
  }

  listConnectionsByPlacement(placementId: string) {
    return this.client.listConnectionsByPlacement({ placementId });
  }

  createPhysicalConnection(
    sourcePlacementId: string,
    sourcePortDefinitionId: string,
    targetPlacementId: string,
    targetPortDefinitionId: string,
  ) {
    return this.client.createPhysicalConnection({
      sourcePlacementId,
      sourcePortDefinitionId,
      targetPlacementId,
      targetPortDefinitionId,
    });
  }

  updatePhysicalConnection(id: string, notes: string) {
    return this.client.updatePhysicalConnection({ id, notes });
  }

  deletePhysicalConnection(id: string) {
    return this.client.deletePhysicalConnection({ id });
  }
}
