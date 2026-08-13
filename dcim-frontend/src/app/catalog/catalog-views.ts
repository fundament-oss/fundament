import { sectionMatcher } from '../shared/section-views';

/** The section's own address. Every view lives under it. */
export const CATALOG_PATH = '/catalog';

/**
 * /catalog, /catalog/all and /catalog/category/server. A catalog entry id is
 * none of those shapes, so it falls through to the detail route.
 */
export const catalogMatcher = sectionMatcher('catalog', ['category']);
