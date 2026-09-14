import { UrlSegment, type UrlMatchResult } from '@angular/router';

/** The section's own address. Every rack lives under it. */
export const RACKS_PATH = '/racks';

/**
 * /racks and /racks/<id>: the list on its own, and the list with one rack open
 * beside it. One route config for both, so opening a rack, or switching data
 * center, is a parameter change on the same page rather than a page that is
 * torn down and built again. Two configs pointing at the same component threw
 * away everything the page had, the chosen data center included.
 *
 * /racks/device/<id> is a page of its own and falls through to the route that
 * renders it.
 */
export function racksMatcher(segments: UrlSegment[]): UrlMatchResult | null {
  if (segments.length === 0 || segments[0].path !== 'racks') return null;
  if (segments.length === 1) return { consumed: segments, posParams: {} };
  if (segments.length === 2 && segments[1].path !== 'device') {
    return { consumed: segments, posParams: { rackId: segments[1] } };
  }
  return null;
}
