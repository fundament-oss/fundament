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

export const ASSET_STATUS_BADGE_CLASS: Record<AssetStatus, string> = {
  deployed: 'bg-teal-50 dark:bg-teal-950 text-teal-700 dark:text-teal-300',
  available: 'bg-green-50 dark:bg-green-950 text-green-700 dark:text-green-300',
  'needs-repair': 'bg-amber-50 dark:bg-amber-950 text-amber-700 dark:text-amber-300',
  decommissioned: 'bg-slate-100 dark:bg-gray-800 text-slate-500 dark:text-gray-400',
  'on-order': 'bg-blue-50 dark:bg-blue-950 text-blue-700 dark:text-blue-300',
  requested: 'bg-purple-50 dark:bg-purple-950 text-purple-700 dark:text-purple-300',
};

export const ASSET_STATUS_DOT_CLASS: Record<AssetStatus, string> = {
  deployed: 'bg-teal-400',
  available: 'bg-green-400',
  'needs-repair': 'bg-amber-400',
  decommissioned: 'bg-slate-400',
  'on-order': 'bg-blue-400',
  requested: 'bg-purple-400',
};
