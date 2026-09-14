import type { AssetStatus } from './inventory';

// Shared status → presentation mappings for assets. Used by the inventory list,
// asset detail, and catalog detail views so the palette stays in sync instead of
// drifting across hand-maintained copies.

// Sentence case, like every other label in this app: "All assets", "Add cable",
// "To do". These six were the only ones still in title case.
export const ASSET_STATUS_LABEL: Record<AssetStatus, string> = {
  deployed: 'Deployed',
  available: 'Available',
  'needs-repair': 'Needs repair',
  decommissioned: 'Decommissioned',
  'on-order': 'On order',
  requested: 'Requested',
};

/**
 * `color` for an `nldd-tag` per asset status.
 *
 * Read as how loudly a state asks for a person. Critical on the one that is
 * actually broken, warning on the one nobody has bought yet, accent on the one
 * lying on a shelf where the next move is yours, success on the one doing its
 * job. On order keeps a quiet blue of its own: it is handled, it just has not
 * arrived, and it is the sixth state where the palette holds five.
 */
export const ASSET_STATUS_TAG_COLOR: Record<AssetStatus, string> = {
  deployed: 'success',
  available: 'accent',
  'needs-repair': 'critical',
  decommissioned: 'neutral',
  'on-order': 'lichtblauw',
  requested: 'warning',
};

function statusList(order: AssetStatus[]): { value: AssetStatus; label: string }[] {
  return order.map((value) => ({ value, label: ASSET_STATUS_LABEL[value] }));
}

/**
 * The states in the order they ask something of you: broken first, then what
 * somebody asked for and still has to be ordered, then what is on its way and
 * might need chasing, then what is ready to use, then what is quietly working,
 * and what is written off last. Requested sits above On order because it is
 * still your move; once it is ordered the wait is the supplier's.
 *
 * What the menu lists, and what the rows are sorted by.
 */
export const ASSET_STATUSES_BY_ATTENTION = statusList([
  'needs-repair',
  'requested',
  'on-order',
  'available',
  'deployed',
  'decommissioned',
]);

/**
 * The order a form offers them in, which is not the same question: there it is
 * about where an asset usually is when somebody writes it down, and that is in
 * use or on the shelf.
 */
export const ASSET_STATUSES = statusList([
  'deployed',
  'available',
  'on-order',
  'requested',
  'needs-repair',
  'decommissioned',
]);
