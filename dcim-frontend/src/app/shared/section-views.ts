import { UrlSegment, type UrlMatchResult } from '@angular/router';

/** A label as it reads in an address: lowercase, spaces closed up. */
export function viewSlug(label: string): string {
  return label.toLowerCase().replace(/\s+/g, '-');
}

/**
 * Builds the matcher for a section whose menu is navigation: one route config
 * for every view, so switching view is a parameter change on the same route and
 * the component keeps what it loaded. Three separate configs pointing at the
 * same component would tear it down and rebuild it on every click in the menu.
 *
 * It claims the section itself and the shapes the menu can produce, and nothing
 * else. That matters where a section also has detail pages: /inventory/<id> is
 * not a view, so the matcher lets it fall through to the route that is.
 *
 * @param section  the first segment, e.g. 'inventory'
 * @param kinds    the view kinds that take a value, e.g. ['status', 'category']
 */
export function sectionMatcher(section: string, kinds: string[]) {
  return (segments: UrlSegment[]): UrlMatchResult | null => {
    if (segments.length === 0 || segments[0].path !== section) return null;

    const rest = segments.slice(1);
    if (rest.length === 0) return { consumed: segments, posParams: {} };
    if (rest.length === 1 && rest[0].path === 'all') {
      return { consumed: segments, posParams: { view: rest[0] } };
    }
    if (rest.length === 2 && kinds.includes(rest[0].path)) {
      return { consumed: segments, posParams: { view: rest[0], value: rest[1] } };
    }
    return null;
  };
}
