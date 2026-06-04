import { AssetCategory } from '../inventory/inventory';
import { DeviceType } from './rack.model';
import { Port, PortType } from '../patch-mapping/cable.model';
import {
  PortType as ProtoPortType,
  PortDirection as ProtoPortDirection,
} from '../../generated/v1/common_pb';
import type { PortDefinition as ProtoPortDefinition } from '../../generated/v1/catalog_pb';

/**
 * Reads the rack height (in U) from a catalog entry's free-form `specs` map.
 * Falls back to 1U when missing or unparseable.
 */
export function parseRackHeight(specs: Record<string, string> | undefined): number {
  if (!specs) return 1;
  const raw = specs['rack_height'] ?? specs['rackHeight'] ?? specs['height'];
  const parsed = raw ? parseInt(raw, 10) : NaN;
  return Number.isFinite(parsed) && parsed > 0 ? parsed : 1;
}

/**
 * Maps a catalog entry's asset category to the rack-diagram device type used
 * for slot colouring. Falls back to `'machine'` for unknown / null categories.
 */
export function categoryToDeviceType(category: AssetCategory | undefined): DeviceType {
  switch (category) {
    case 'Switch':
      return 'switch';
    case 'Power':
    case 'PSU':
      return 'pdu';
    default:
      return 'machine';
  }
}

/**
 * Maps a catalog port definition's proto port type to the cabling UI's port
 * type. Returns null for non-cabling ports (module slots, drive bays), which
 * the patch/cabling views don't represent.
 */
function cablePortType(portType: ProtoPortType, direction: ProtoPortDirection): PortType | null {
  switch (portType) {
    case ProtoPortType.NETWORK:
      return 'network-interface';
    case ProtoPortType.POWER_IN:
      return 'power-port';
    case ProtoPortType.POWER_OUT:
      return 'power-outlet';
    case ProtoPortType.CONSOLE:
      return direction === ProtoPortDirection.OUT ? 'console-server-port' : 'console-port';
    default:
      return null;
  }
}

/**
 * Maps a catalog port definition onto the cabling UI `Port` model for a given
 * device (placement). Returns null when the definition is not a cabling port.
 */
export function cablePortFromDefinition(pd: ProtoPortDefinition, deviceId: string): Port | null {
  const type = cablePortType(pd.portType, pd.direction);
  if (!type) return null;
  return { id: pd.id, deviceId, name: pd.name, type };
}

/**
 * Reverse of {@link cablePortFromDefinition}: maps a cabling UI port type onto
 * the catalog port-definition `portType`/`direction` enum keys, so ports added
 * from the cabling views can be written back as catalog port definitions.
 */
export function cablePortTypeToDefinition(type: PortType): { portType: string; direction: string } {
  switch (type) {
    case 'network-interface':
      return { portType: 'network', direction: 'bidir' };
    case 'power-port':
      return { portType: 'power_in', direction: 'in' };
    case 'power-outlet':
      return { portType: 'power_out', direction: 'out' };
    case 'console-port':
      return { portType: 'console', direction: 'in' };
    case 'console-server-port':
      return { portType: 'console', direction: 'out' };
    default:
      throw new Error(`Unhandled cabling port type: ${type as string}`);
  }
}
