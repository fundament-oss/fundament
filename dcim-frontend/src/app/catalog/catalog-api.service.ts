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
      partNumber: d.partNumber,
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
      portType: CatalogApiService.fromProtoPortType(p.portType),
      ...(p.mediaType ? { mediaType: p.mediaType } : {}),
      ...(speedGbps != null ? { speedGbps } : {}),
      ...(p.maxPowerW ? { powerWatts: p.maxPowerW } : {}),
    };
  }

  private static fromProtoPortType(t: PortType): string {
    const map: Record<number, string> = {
      [PortType.NETWORK]: 'network',
      [PortType.POWER_IN]: 'power_in',
      [PortType.POWER_OUT]: 'power_out',
      [PortType.SLOT]: 'slot',
      [PortType.BAY]: 'bay',
      [PortType.CONSOLE]: 'console',
    };
    return map[t] ?? '';
  }

  private static toProtoPortType(key: string): PortType {
    const map: Record<string, PortType> = {
      network: PortType.NETWORK,
      power_in: PortType.POWER_IN,
      power_out: PortType.POWER_OUT,
      slot: PortType.SLOT,
      bay: PortType.BAY,
      console: PortType.CONSOLE,
    };
    return map[key] ?? PortType.UNSPECIFIED;
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
      [ProtoCategory.STORAGE]: 'Storage',
      [ProtoCategory.COOLING]: 'Cooling',
      [ProtoCategory.FIREWALL]: 'Firewall',
      [ProtoCategory.KVM]: 'KVM',
      [ProtoCategory.GPU]: 'GPU',
      [ProtoCategory.TRANSCEIVER]: 'Transceiver',
      [ProtoCategory.OTHER]: 'Other',
    };
    return map[cat] ?? 'Other';
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
      Transceiver: ProtoCategory.TRANSCEIVER,
      Other: ProtoCategory.OTHER,
    };
    return map[cat] ?? ProtoCategory.UNSPECIFIED;
  }

  // ── API methods ───────────────────────────────────────────────────────────

  listCatalog(search?: string) {
    return this.client.listCatalog(search ? { search } : {});
  }

  getCatalogEntry(id: string) {
    return this.client.getCatalogEntry({ id });
  }

  createCatalogEntry(entry: CatalogEntry) {
    return this.client.createCatalogEntry({
      manufacturer: entry.manufacturer,
      model: entry.model,
      partNumber: entry.partNumber ?? '',
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
      portType: CatalogApiService.toProtoPortType(pd.portType),
      mediaType: pd.mediaType ?? '',
      ...(pd.speedGbps != null ? { speed: String(pd.speedGbps) } : {}),
      ...(pd.powerWatts != null ? { maxPowerW: pd.powerWatts } : {}),
    });
  }

  updatePortDefinition(pd: PortDefinition) {
    return this.client.updatePortDefinition({
      id: pd.id,
      name: pd.name,
      portType: CatalogApiService.toProtoPortType(pd.portType),
      mediaType: pd.mediaType ?? '',
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
