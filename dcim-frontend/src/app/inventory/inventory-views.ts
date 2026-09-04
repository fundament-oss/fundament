import { sectionMatcher, viewSlug } from '../shared/section-views';
import { CATEGORIES } from '../shared/asset-category';
import { ASSET_STATUS_LABEL } from './asset-status';

/** The section's own address. Every view lives under it. */
export const INVENTORY_PATH = '/inventory';

/**
 * /inventory, /inventory/all, /inventory/status/deployed and
 * /inventory/category/server. An asset id is none of those shapes, so it falls
 * through to the detail route.
 */
export const inventoryMatcher = sectionMatcher('inventory', ['status', 'category']);

/** True for an address that is one of the section's views, not one asset. */
export function isInventoryView(url: string): boolean {
  const [path] = url.split('?');
  if (path === INVENTORY_PATH || path === `${INVENTORY_PATH}/all`) return true;
  const rest = path.startsWith(`${INVENTORY_PATH}/`) ? path.slice(INVENTORY_PATH.length + 1) : '';
  const [kind] = rest.split('/');
  return kind === 'status' || kind === 'category';
}

/**
 * The name of a view as the menu writes it. The page of one asset uses it for
 * the way back, so the button says which list you left rather than repeating
 * the section name that is already in the menu beside it.
 */
export function inventoryViewTitle(url: string): string {
  const [path] = url.split('?');
  const rest = path.startsWith(`${INVENTORY_PATH}/`) ? path.slice(INVENTORY_PATH.length + 1) : '';
  const [kind, value] = rest.split('/');
  if (kind === 'status') {
    const label = Object.entries(ASSET_STATUS_LABEL).find(([status]) => viewSlug(status) === value);
    return label ? label[1] : 'All assets';
  }
  if (kind === 'category') {
    return CATEGORIES.find((category) => viewSlug(category) === value) ?? 'All assets';
  }
  return 'All assets';
}
