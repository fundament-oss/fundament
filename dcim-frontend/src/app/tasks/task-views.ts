import { UrlSegment, type UrlMatchResult } from '@angular/router';

/** The section's own address. Every view lives under it. */
export const TASKS_PATH = '/tasks';

/**
 * One route config for all of the views, rather than one per shape of address.
 * Switching view is then a parameter change on the same route, so the component
 * keeps the tasks it loaded and the sheet you may have open; three configs
 * pointing at the same component would tear it down and rebuild it on every
 * click in the menu.
 *
 * It swallows at most two segments after `tasks`, which is what the deepest
 * view needs: `/tasks/priority/urgent`.
 */
export function tasksMatcher(segments: UrlSegment[]): UrlMatchResult | null {
  if (segments.length === 0 || segments[0].path !== 'tasks') return null;

  const rest = segments.slice(1);
  if (rest.length > 2) return null;

  const posParams: Record<string, UrlSegment> = {};
  if (rest[0]) posParams['view'] = rest[0];
  if (rest[1]) posParams['value'] = rest[1];
  return { consumed: segments, posParams };
}

/** A label as it reads in an address: lowercase, spaces closed up. */
export function viewSlug(label: string): string {
  return label.toLowerCase().replace(/\s+/g, '-');
}
