import { UrlSegment, type UrlMatchResult } from '@angular/router';

/** The section's own address. Every round lives under it. */
export const ROUNDS_PATH = '/rounds';

/**
 * One route config for the list and for a round on it, the same arrangement the
 * tasks section uses: switching round is then a parameter change rather than a
 * new component, so the page keeps what it loaded.
 *
 * It swallows at most three segments after `rounds`, which is what a round's
 * address is made of: who, where, and when. Today's round leaves the day off.
 */
export function roundsMatcher(segments: UrlSegment[]): UrlMatchResult | null {
  if (segments.length === 0 || segments[0].path !== 'rounds') return null;

  const rest = segments.slice(1);
  if (rest.length > 3) return null;

  const posParams: Record<string, UrlSegment> = {};
  if (rest[0]) posParams['person'] = rest[0];
  if (rest[1]) posParams['datacenter'] = rest[1];
  if (rest[2]) posParams['day'] = rest[2];
  return { consumed: segments, posParams };
}

/** A person in an address: their name, lowercase, spaces closed up. Readable
 *  beats unique here, and the id is accepted just as well. */
export const personSlug = (name: string): string => name.toLowerCase().replace(/\s+/g, '-');
