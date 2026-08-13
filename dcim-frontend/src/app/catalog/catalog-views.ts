import { sectionMatcher, viewSlug } from '../shared/section-views';
import { CATEGORIES } from '../shared/asset-category';

/** The section's own address. Every view lives under it. */
export const CATALOG_PATH = '/catalog';

/**
 * /catalog, /catalog/all and /catalog/category/server. A catalog entry id is
 * none of those shapes, so it falls through to the detail route.
 */
export const catalogMatcher = sectionMatcher('catalog', ['category']);

/** True for an address that is one of the section's views, not one product. */
export function isCatalogView(url: string): boolean {
  const [path] = url.split('?');
  if (path === CATALOG_PATH || path === `${CATALOG_PATH}/all`) return true;
  const rest = path.startsWith(`${CATALOG_PATH}/`) ? path.slice(CATALOG_PATH.length + 1) : '';
  return rest.split('/')[0] === 'category';
}

/**
 * The name of a view as the menu writes it. The page of one product uses it for
 * the way back, so the button says which list you left rather than repeating
 * the section name that is already in the menu beside it.
 */
export function catalogViewTitle(url: string): string {
  const [path] = url.split('?');
  const rest = path.startsWith(`${CATALOG_PATH}/`) ? path.slice(CATALOG_PATH.length + 1) : '';
  const [kind, value] = rest.split('/');
  if (kind !== 'category') return 'All products';
  return CATEGORIES.find((category) => viewSlug(category) === value) ?? 'All products';
}
