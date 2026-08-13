import { sectionMatcher } from '../shared/section-views';

/** The section's own address. Every view lives under it. */
export const INVENTORY_PATH = '/inventory';

/**
 * /inventory, /inventory/all, /inventory/status/deployed and
 * /inventory/category/server. An asset id is none of those shapes, so it falls
 * through to the detail route.
 */
export const inventoryMatcher = sectionMatcher('inventory', ['status', 'category']);
