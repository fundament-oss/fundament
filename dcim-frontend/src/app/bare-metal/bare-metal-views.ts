import { type ParamMap, UrlSegment, type UrlMatchResult } from '@angular/router';

/** The section's own address. Every view of the metal-stack estate lives under
 *  it. */
export const BARE_METAL_PATH = '/bare-metal';

/** What the section can show: what the hardware can hold, what is in it, how it
 *  is wired, and what the control plane is doing. */
export type BareMetalView =
  { kind: 'capacity' } | { kind: 'machines' } | { kind: 'network' } | { kind: 'api' };

/**
 * /bare-metal/capacity, /bare-metal/machines, /bare-metal/network and
 * /bare-metal/api: one route config for all four, as the Platform section
 * does, so moving between them is a parameter change on the same page.
 *
 * /bare-metal itself shows capacity too, as /inventory shows all assets, but
 * the menu links to /bare-metal/capacity. The shell reads the section's own
 * address as depth 1, the menu: once the columns collapse to one, a link to
 * /bare-metal would leave you on the menu rather than open the view.
 */
export function bareMetalMatcher(segments: UrlSegment[]): UrlMatchResult | null {
  if (segments.length === 0 || segments[0].path !== 'bare-metal') return null;
  if (segments.length === 1) return { consumed: segments, posParams: {} };
  if (segments.length !== 2) return null;
  const view = segments[1].path;
  if (view !== 'capacity' && view !== 'machines' && view !== 'network' && view !== 'api') {
    return null;
  }
  return { consumed: segments, posParams: { view: segments[1] } };
}

/** Reads the view back from the parameters the matcher set. */
export function bareMetalViewOf(params: ParamMap): BareMetalView {
  const view = params.get('view');
  if (view === 'machines') return { kind: 'machines' };
  if (view === 'network') return { kind: 'network' };
  if (view === 'api') return { kind: 'api' };
  return { kind: 'capacity' };
}

/** The address of a view, the inverse of the matcher. */
export function bareMetalPath(view: BareMetalView): string {
  switch (view.kind) {
    case 'capacity':
      return `${BARE_METAL_PATH}/capacity`;
    case 'machines':
      return `${BARE_METAL_PATH}/machines`;
    case 'network':
      return `${BARE_METAL_PATH}/network`;
    case 'api':
      return `${BARE_METAL_PATH}/api`;
    default: {
      const unhandled: never = view;
      throw new Error(`unhandled bare metal view ${JSON.stringify(unhandled)}`);
    }
  }
}
