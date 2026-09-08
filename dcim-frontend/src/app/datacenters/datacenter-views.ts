import { UrlSegment, type UrlMatchResult } from '@angular/router';

/** The section's own address. Every data center lives under it. */
export const DATA_CENTERS_PATH = '/data-centers';

/**
 * /data-centers and /data-centers/ams1: the list on its own, and the list with
 * one data center open beside it. One route config for both, so picking one is
 * a parameter change on the same route and the page keeps what it loaded. Two
 * configs pointing at the same component would tear it down and build it again
 * on every click in the menu.
 *
 * /data-centers/ams1/layout is the same page with the layout editor open over
 * it. The editor is a sheet and not a page of its own, but it still has an
 * address: opening it puts that address in the bar, so it can be linked to and
 * so the back button closes it. Following such a link lands you on the data
 * center with the editor already open, which is where the link was made.
 */
export function dataCentersMatcher(segments: UrlSegment[]): UrlMatchResult | null {
  if (segments.length === 0 || segments[0].path !== 'data-centers') return null;
  if (segments.length === 1) return { consumed: segments, posParams: {} };
  if (segments.length === 2) return { consumed: segments, posParams: { slug: segments[1] } };
  if (segments.length === 3 && segments[2].path === 'layout') {
    return { consumed: segments, posParams: { slug: segments[1], overlay: segments[2] } };
  }
  return null;
}
