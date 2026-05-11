import { Injectable, inject } from '@angular/core';
import { AssetStatus as ProtoStatus } from '../../generated/v1/common_pb';
import type { Asset as ProtoAsset } from '../../generated/v1/asset_pb';
import type { Asset, AssetStatus } from './inventory';
import { ASSET_CLIENT } from '../../connect/tokens';

@Injectable({ providedIn: 'root' })
export default class InventoryApiService {
  private readonly assetClient = inject(ASSET_CLIENT);

  static mapAsset(a: ProtoAsset): Partial<Asset> {
    return {
      id: a.id,
      assetTag: a.assetTag,
      status: InventoryApiService.fromProtoStatus(a.status),
      notes: a.notes,
    };
  }

  private static fromProtoStatus(s: ProtoStatus): AssetStatus {
    const map: Record<number, AssetStatus> = {
      [ProtoStatus.DEPLOYED]: 'deployed',
      [ProtoStatus.AVAILABLE]: 'available',
      [ProtoStatus.DECOMMISSIONED]: 'decommissioned',
      [ProtoStatus.NEEDS_REPAIR]: 'needs-repair',
      [ProtoStatus.ON_ORDER]: 'on-order',
      [ProtoStatus.REQUESTED]: 'requested',
    };
    return map[s] ?? 'available';
  }

  private static toProtoStatus(s: AssetStatus): ProtoStatus {
    const map: Record<AssetStatus, ProtoStatus> = {
      deployed: ProtoStatus.DEPLOYED,
      available: ProtoStatus.AVAILABLE,
      decommissioned: ProtoStatus.DECOMMISSIONED,
      'needs-repair': ProtoStatus.NEEDS_REPAIR,
      'on-order': ProtoStatus.ON_ORDER,
      requested: ProtoStatus.REQUESTED,
    };
    return map[s] ?? ProtoStatus.AVAILABLE;
  }

  createAsset(asset: Asset) {
    return this.assetClient.createAsset({
      deviceCatalogId: '',
      status: InventoryApiService.toProtoStatus(asset.status),
      assetTag: asset.assetTag,
      notes: asset.notes,
    });
  }

  updateAsset(asset: Asset) {
    return this.assetClient.updateAsset({
      id: asset.id,
      status: InventoryApiService.toProtoStatus(asset.status),
      assetTag: asset.assetTag,
      notes: asset.notes,
    });
  }

  deleteAsset(id: string) {
    return this.assetClient.deleteAsset({ id });
  }
}
