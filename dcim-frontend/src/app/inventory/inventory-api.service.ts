import { Injectable, inject } from '@angular/core';
import {
  AssetCategory as ProtoCategory,
  AssetStatus as ProtoStatus,
  SortDirection,
} from '../../generated/v1/common_pb';
import { AssetSortField } from '../../generated/v1/asset_pb';
import type { Asset as ProtoAsset } from '../../generated/v1/asset_pb';
import type { Asset, AssetCategory, AssetStatus, CatalogEntry } from './inventory';
import { ASSET_CLIENT } from '../../connect/tokens';

export interface ListAssetsOptions {
  search?: string;
  status: AssetStatus | 'all';
  category: AssetCategory | 'all';
  sortDirection: 'asc' | 'desc';
}

@Injectable({ providedIn: 'root' })
export default class InventoryApiService {
  private readonly assetClient = inject(ASSET_CLIENT);

  /**
   * Maps an API asset onto the UI Asset model. The API asset only carries a
   * device_catalog_id, so model and category are resolved from the catalog.
   */
  static mapAsset(a: ProtoAsset, catalog: Map<string, CatalogEntry>): Asset {
    const entry = catalog.get(a.deviceCatalogId);
    return {
      id: a.id,
      deviceCatalogId: a.deviceCatalogId,
      model: entry?.model ?? 'Unknown device',
      category: entry?.category ?? 'Other',
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

  private static toProtoCategory(cat: AssetCategory): ProtoCategory {
    const map: Record<AssetCategory, ProtoCategory> = {
      Server: ProtoCategory.SERVER,
      Switch: ProtoCategory.SWITCH,
      Storage: ProtoCategory.STORAGE,
      Power: ProtoCategory.PDU,
      Firewall: ProtoCategory.FIREWALL,
      Cooling: ProtoCategory.COOLING,
      KVM: ProtoCategory.KVM,
      Memory: ProtoCategory.DIMM,
      Disk: ProtoCategory.DISK,
      NIC: ProtoCategory.NIC,
      PSU: ProtoCategory.POWER_SUPPLY,
      CPU: ProtoCategory.CPU,
      GPU: ProtoCategory.GPU,
      Transceiver: ProtoCategory.SFP,
      Other: ProtoCategory.OTHER,
    };
    return map[cat] ?? ProtoCategory.UNSPECIFIED;
  }

  listAssets(opts: ListAssetsOptions) {
    return this.assetClient.listAssets({
      sortBy: AssetSortField.STATUS,
      sortDirection: opts.sortDirection === 'desc' ? SortDirection.DESC : SortDirection.ASC,
      ...(opts.search ? { search: opts.search } : {}),
      ...(opts.status !== 'all'
        ? { statusFilter: InventoryApiService.toProtoStatus(opts.status) }
        : {}),
      ...(opts.category !== 'all'
        ? { categoryFilter: InventoryApiService.toProtoCategory(opts.category) }
        : {}),
    });
  }

  getAsset(id: string) {
    return this.assetClient.getAsset({ id });
  }

  getAssetStats() {
    return this.assetClient.getAssetStats({});
  }

  createAsset(asset: Asset) {
    return this.assetClient.createAsset({
      deviceCatalogId: asset.deviceCatalogId ?? '',
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
