import { AssetCategory } from '../inventory/inventory';
import { DeviceType } from './rack.model';

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
