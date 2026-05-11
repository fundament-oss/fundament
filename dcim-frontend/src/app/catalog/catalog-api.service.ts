import { Injectable, inject } from '@angular/core';
import { AssetCategory as ProtoCategory, PortType } from '../../generated/v1/common_pb';
import type {
  DeviceCatalog,
  PortDefinition as ProtoPortDef,
  PortCompatibility as ProtoPortCompat,
} from '../../generated/v1/catalog_pb';
import type {
  CatalogEntry,
  PortDefinition,
  PortCompatibility,
  AssetCategory,
} from '../inventory/inventory';
import { CATALOG_CLIENT } from '../../connect/tokens';

@Injectable({ providedIn: 'root' })
export default class CatalogApiService {
  private readonly client = inject(CATALOG_CLIENT);

  // ── Mappers ───────────────────────────────────────────────────────────────

  static mapCatalogEntry(d: DeviceCatalog): CatalogEntry {
    return {
      id: d.id,
      model: d.model,
      manufacturer: d.manufacturer,
      category: CatalogApiService.fromProtoCategory(d.category),
      specs: d.specs as Record<string, string>,
    };
  }

  static mapPortDefinition(p: ProtoPortDef): PortDefinition {
    const speedGbps = p.speed ? parseFloat(p.speed) || undefined : undefined;
    return {
      id: p.id,
      catalogEntryId: p.deviceCatalogId,
      name: p.name,
      portType: String(p.portType),
      ...(speedGbps != null ? { speedGbps } : {}),
      ...(p.maxPowerW ? { powerWatts: p.maxPowerW } : {}),
    };
  }

  static mapPortCompatibility(c: ProtoPortCompat): PortCompatibility {
    return {
      id: `${c.portDefinitionId}:${c.compatibleCatalogId}`,
      portDefinitionId: c.portDefinitionId,
      compatibleCatalogEntryId: c.compatibleCatalogId,
    };
  }

  private static fromProtoCategory(cat: ProtoCategory): AssetCategory {
    const map: Record<number, AssetCategory> = {
      [ProtoCategory.SERVER]: 'Server',
      [ProtoCategory.SWITCH]: 'Switch',
      [ProtoCategory.PDU]: 'Power',
      [ProtoCategory.PATCH_PANEL]: 'Other',
      [ProtoCategory.SFP]: 'Transceiver',
      [ProtoCategory.NIC]: 'NIC',
      [ProtoCategory.CPU]: 'CPU',
      [ProtoCategory.DIMM]: 'Memory',
      [ProtoCategory.DISK]: 'Disk',
      [ProtoCategory.CABLE]: 'Other',
      [ProtoCategory.ADAPTER]: 'Other',
      [ProtoCategory.POWER_SUPPLY]: 'PSU',
      [ProtoCategory.CABLE_MANAGER]: 'Other',
      [ProtoCategory.CONSOLE_SERVER]: 'Other',
    };
    return map[cat] ?? 'Other';
  }

  private static toProtoCategory(cat: AssetCategory): ProtoCategory {
    const map: Partial<Record<AssetCategory, ProtoCategory>> = {
      Server: ProtoCategory.SERVER,
      Switch: ProtoCategory.SWITCH,
      Power: ProtoCategory.PDU,
      NIC: ProtoCategory.NIC,
      CPU: ProtoCategory.CPU,
      Memory: ProtoCategory.DIMM,
      Disk: ProtoCategory.DISK,
      PSU: ProtoCategory.POWER_SUPPLY,
      Transceiver: ProtoCategory.SFP,
    };
    return map[cat] ?? ProtoCategory.UNSPECIFIED;
  }

  // ── API methods ───────────────────────────────────────────────────────────

  listCatalog() {
    return this.client.listCatalog({});
  }

  createCatalogEntry(entry: CatalogEntry) {
    return this.client.createCatalogEntry({
      manufacturer: entry.manufacturer,
      model: entry.model,
      partNumber: '',
      category: CatalogApiService.toProtoCategory(entry.category),
      formFactor: '',
      specs: entry.specs,
    });
  }

  updateCatalogEntry(entry: CatalogEntry) {
    return this.client.updateCatalogEntry({
      id: entry.id,
      manufacturer: entry.manufacturer,
      model: entry.model,
      category: CatalogApiService.toProtoCategory(entry.category),
      specs: entry.specs,
    });
  }

  deleteCatalogEntry(id: string) {
    return this.client.deleteCatalogEntry({ id });
  }

  listPortDefinitions(deviceCatalogId: string) {
    return this.client.listPortDefinitions({ deviceCatalogId });
  }

  createPortDefinition(pd: PortDefinition) {
    return this.client.createPortDefinition({
      deviceCatalogId: pd.catalogEntryId,
      name: pd.name,
      portType: PortType.UNSPECIFIED,
      mediaType: pd.portType,
      ...(pd.speedGbps != null ? { speed: String(pd.speedGbps) } : {}),
      ...(pd.powerWatts != null ? { maxPowerW: pd.powerWatts } : {}),
    });
  }

  updatePortDefinition(pd: PortDefinition) {
    return this.client.updatePortDefinition({
      id: pd.id,
      name: pd.name,
      mediaType: pd.portType,
      ...(pd.speedGbps != null ? { speed: String(pd.speedGbps) } : {}),
      ...(pd.powerWatts != null ? { maxPowerW: pd.powerWatts } : {}),
    });
  }

  deletePortDefinition(id: string) {
    return this.client.deletePortDefinition({ id });
  }

  listPortCompatibilities(portDefinitionId: string) {
    return this.client.listPortCompatibilities({ portDefinitionId });
  }

  createPortCompatibility(portDefinitionId: string, compatibleCatalogId: string) {
    return this.client.createPortCompatibility({ portDefinitionId, compatibleCatalogId });
  }

  deletePortCompatibility(portDefinitionId: string, compatibleCatalogId: string) {
    return this.client.deletePortCompatibility({ portDefinitionId, compatibleCatalogId });
  }
}
