import { Injectable, inject } from '@angular/core';
import type { PhysicalConnection as ProtoConn } from '../../generated/v1/connection_pb';
import {
  CableType as ProtoCableType,
  CableStatus as ProtoCableStatus,
  CableColor as ProtoCableColor,
} from '../../generated/v1/common_pb';
import type { Cable, CableColor, CableStatus, CableType, Port } from './cable.model';
import { PHYSICAL_CONNECTION_CLIENT } from '../../connect/tokens';

/** Lookups for resolving a connection's placement/port ids into display data. */
export interface CableLookups {
  /** placement id -> device (display) name */
  deviceNameById: Map<string, string>;
  /** port definition id -> UI port (name + type) */
  portById: Map<string, Port>;
}

@Injectable({ providedIn: 'root' })
export default class PatchMappingApiService {
  private readonly client = inject(PHYSICAL_CONNECTION_CLIENT);

  // ── Enum mapping (UI union <-> proto enum) ─────────────────────────────────

  static cableTypeToProto(t: CableType): ProtoCableType {
    switch (t) {
      case 'cat5e':
        return ProtoCableType.CAT5E;
      case 'cat6':
        return ProtoCableType.CAT6;
      case 'cat6a':
        return ProtoCableType.CAT6A;
      case 'cat7':
        return ProtoCableType.CAT7;
      case 'cat8':
        return ProtoCableType.CAT8;
      case 'dac':
        return ProtoCableType.DAC;
      case 'aoc':
        return ProtoCableType.AOC;
      case 'mmf':
        return ProtoCableType.MMF;
      case 'smf':
        return ProtoCableType.SMF;
      case 'power':
        return ProtoCableType.POWER;
      case 'console':
        return ProtoCableType.CONSOLE;
      case 'usb':
        return ProtoCableType.USB;
      case 'other':
        return ProtoCableType.OTHER;
      default:
        return ProtoCableType.UNSPECIFIED;
    }
  }

  static cableTypeFromProto(t: ProtoCableType): CableType {
    switch (t) {
      case ProtoCableType.CAT5E:
        return 'cat5e';
      case ProtoCableType.CAT6:
        return 'cat6';
      case ProtoCableType.CAT6A:
        return 'cat6a';
      case ProtoCableType.CAT7:
        return 'cat7';
      case ProtoCableType.CAT8:
        return 'cat8';
      case ProtoCableType.DAC:
        return 'dac';
      case ProtoCableType.AOC:
        return 'aoc';
      case ProtoCableType.MMF:
        return 'mmf';
      case ProtoCableType.SMF:
        return 'smf';
      case ProtoCableType.POWER:
        return 'power';
      case ProtoCableType.CONSOLE:
        return 'console';
      case ProtoCableType.USB:
        return 'usb';
      default:
        return 'other';
    }
  }

  static cableStatusToProto(s: CableStatus): ProtoCableStatus {
    switch (s) {
      case 'planned':
        return ProtoCableStatus.PLANNED;
      case 'connected':
        return ProtoCableStatus.CONNECTED;
      case 'decommissioned':
        return ProtoCableStatus.DECOMMISSIONED;
      default:
        return ProtoCableStatus.UNSPECIFIED;
    }
  }

  static cableStatusFromProto(s: ProtoCableStatus): CableStatus {
    switch (s) {
      case ProtoCableStatus.PLANNED:
        return 'planned';
      case ProtoCableStatus.DECOMMISSIONED:
        return 'decommissioned';
      default:
        return 'connected';
    }
  }

  static cableColorToProto(c: CableColor | undefined): ProtoCableColor {
    switch (c) {
      case 'dark-grey':
        return ProtoCableColor.DARK_GREY;
      case 'light-grey':
        return ProtoCableColor.LIGHT_GREY;
      case 'red':
        return ProtoCableColor.RED;
      case 'green':
        return ProtoCableColor.GREEN;
      case 'blue':
        return ProtoCableColor.BLUE;
      case 'yellow':
        return ProtoCableColor.YELLOW;
      case 'purple':
        return ProtoCableColor.PURPLE;
      case 'orange':
        return ProtoCableColor.ORANGE;
      case 'teal':
        return ProtoCableColor.TEAL;
      case 'white':
        return ProtoCableColor.WHITE;
      default:
        return ProtoCableColor.UNSPECIFIED;
    }
  }

  static cableColorFromProto(c: ProtoCableColor): CableColor | undefined {
    switch (c) {
      case ProtoCableColor.DARK_GREY:
        return 'dark-grey';
      case ProtoCableColor.LIGHT_GREY:
        return 'light-grey';
      case ProtoCableColor.RED:
        return 'red';
      case ProtoCableColor.GREEN:
        return 'green';
      case ProtoCableColor.BLUE:
        return 'blue';
      case ProtoCableColor.YELLOW:
        return 'yellow';
      case ProtoCableColor.PURPLE:
        return 'purple';
      case ProtoCableColor.ORANGE:
        return 'orange';
      case ProtoCableColor.TEAL:
        return 'teal';
      case ProtoCableColor.WHITE:
        return 'white';
      default:
        return undefined;
    }
  }

  /** Maps an API connection onto the UI Cable model, resolving display data via lookups. */
  static mapConnection(c: ProtoConn, dcId: string, lookups: CableLookups): Cable {
    const aPort = lookups.portById.get(c.sourcePortDefinitionId);
    const bPort = lookups.portById.get(c.targetPortDefinitionId);
    return {
      id: c.id,
      dcId,
      aSide: {
        deviceId: c.sourcePlacementId,
        deviceName: lookups.deviceNameById.get(c.sourcePlacementId) ?? c.sourcePlacementId,
        portId: c.sourcePortDefinitionId,
        portName: aPort?.name ?? c.sourcePortDefinitionId,
        portType: aPort?.type ?? 'network-interface',
      },
      bSide: {
        deviceId: c.targetPlacementId,
        deviceName: lookups.deviceNameById.get(c.targetPlacementId) ?? c.targetPlacementId,
        portId: c.targetPortDefinitionId,
        portName: bPort?.name ?? c.targetPortDefinitionId,
        portType: bPort?.type ?? 'network-interface',
      },
      type: PatchMappingApiService.cableTypeFromProto(c.cableType),
      status: PatchMappingApiService.cableStatusFromProto(c.status),
      color: PatchMappingApiService.cableColorFromProto(c.color),
      label: c.label || undefined,
    };
  }

  // ── CRUD ───────────────────────────────────────────────────────────────────

  listConnectionsByPlacement(placementId: string) {
    return this.client.listConnectionsByPlacement({ placementId });
  }

  createCable(cable: Cable) {
    return this.client.createPhysicalConnection({
      sourcePlacementId: cable.aSide.deviceId,
      sourcePortDefinitionId: cable.aSide.portId,
      targetPlacementId: cable.bSide.deviceId,
      targetPortDefinitionId: cable.bSide.portId,
      cableType: PatchMappingApiService.cableTypeToProto(cable.type),
      status: PatchMappingApiService.cableStatusToProto(cable.status),
      color: PatchMappingApiService.cableColorToProto(cable.color),
      label: cable.label ?? '',
    });
  }

  updateCable(cable: Cable) {
    return this.client.updatePhysicalConnection({
      id: cable.id,
      cableType: PatchMappingApiService.cableTypeToProto(cable.type),
      status: PatchMappingApiService.cableStatusToProto(cable.status),
      color: PatchMappingApiService.cableColorToProto(cable.color),
      label: cable.label ?? '',
    });
  }

  deletePhysicalConnection(id: string) {
    return this.client.deletePhysicalConnection({ id });
  }
}
