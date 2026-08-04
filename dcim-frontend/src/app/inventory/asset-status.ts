import type { AssetStatus } from './inventory';

// Shared status → presentation mappings for assets. Used by the inventory list,
// asset detail, and catalog detail views so the palette stays in sync instead of
// drifting across hand-maintained copies.

export const ASSET_STATUS_LABEL: Record<AssetStatus, string> = {
  deployed: 'Deployed',
  available: 'Available',
  'needs-repair': 'Needs Repair',
  decommissioned: 'Decommissioned',
  'on-order': 'On Order',
  requested: 'Requested',
};

/** `color` for an `nldd-tag` per asset status. */
export const ASSET_STATUS_TAG_COLOR: Record<AssetStatus, string> = {
  deployed: 'mintgroen',
  available: 'success',
  'needs-repair': 'warning',
  decommissioned: 'neutral',
  'on-order': 'lichtblauw',
  requested: 'paars',
};
