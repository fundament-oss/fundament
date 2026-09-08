import { sectionMatcher } from '../shared/section-views';

/** The section's own address. Every view lives under it. */
export const PATCH_MAPPING_PATH = '/patch-mapping';

/**
 * /patch-mapping, /patch-mapping/all, /patch-mapping/status/planned,
 * /patch-mapping/type/cat6, /patch-mapping/color/blue and
 * /patch-mapping/device/<id>.
 */
export const patchMappingMatcher = sectionMatcher('patch-mapping', [
  'status',
  'type',
  'color',
  'device',
]);
