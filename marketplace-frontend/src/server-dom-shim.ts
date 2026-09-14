/**
 * Minimal browser globals for server-side rendering.
 *
 * Angular renders against its own DOM implementation and deliberately does not
 * publish browser globals on `globalThis`. Lit ships an SSR DOM shim of its own
 * (`HTMLElement`, `customElements`), so most `@nldd/design-system` components
 * import cleanly under Node, but a few reach for `matchMedia` at module scope —
 * `nldd-tooltip` and `nldd-popover` cache a `(pointer: coarse)` query, which
 * takes `nldd-icon-button` and `nldd-search-field` down with them.
 *
 * This module must be imported before anything that pulls in the design system,
 * so it is the first import of both server entry points. It only fills in what
 * is missing: in the browser these globals already exist and nothing is patched.
 */

interface MinimalMediaQueryList {
  matches: boolean;
  media: string;
  onchange: null;
  addEventListener: () => void;
  removeEventListener: () => void;
  addListener: () => void;
  removeListener: () => void;
  dispatchEvent: () => boolean;
}

const noop = () => {};

// The server has no viewport and no pointer, so every query is a non-match.
// Components that read one of these at construction get the desktop/fine-pointer
// branch, which is also what the client re-evaluates on hydration.
function serverMatchMedia(media: string): MinimalMediaQueryList {
  return {
    matches: false,
    media,
    onchange: null,
    addEventListener: noop,
    removeEventListener: noop,
    addListener: noop,
    removeListener: noop,
    dispatchEvent: () => false,
  };
}

const globals = globalThis as Record<string, unknown>;

if (typeof globals['matchMedia'] !== 'function') {
  globals['matchMedia'] = serverMatchMedia;
}
