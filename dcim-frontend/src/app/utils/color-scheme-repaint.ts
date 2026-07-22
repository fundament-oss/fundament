/**
 * Chromium caches paint tiles for composited scroll layers and does not always
 * invalidate them when a `color-scheme` flip re-resolves `light-dark()` colors.
 * The NLDD design system resolves every color token through `light-dark()`, so
 * a design system color inside a scroll container keeps its pre-toggle value
 * after switching themes — e.g. the active sidebar item staying light blue
 * after switching to dark mode.
 *
 * Mark scroll containers that hold design system colors with
 * `data-scheme-repaint`, and call this right after flipping the theme.
 *
 * This mirrors `forceScrollLayerRepaint` from the design system's own
 * `dist/utilities/color-scheme-repaint`, which is not reachable through the
 * package's `exports` map. Check upstream before changing the approach here,
 * and drop this file if that utility ever gets a public subpath export.
 *
 * Caveat: the display:none -> reflow -> display cycle fires any ResizeObserver
 * watching a marked element (it reports 0x0, then the original size). Harmless
 * for the containers marked today, since nothing observes them, but audit for
 * resize-driven side effects before adding `data-scheme-repaint` elsewhere.
 */
export default function repaintSchemeSensitiveLayers(): void {
  const elements = document.querySelectorAll<HTMLElement>('[data-scheme-repaint]');
  if (elements.length === 0) {
    return;
  }

  // Hiding an element blurs anything focused inside it, so remember where focus
  // was and put it back below.
  const previouslyFocused = document.activeElement;

  /* eslint-disable no-param-reassign -- the callback binding is a queried DOM
     node this helper mutates on purpose. Scoped to the loop; a for-of would
     avoid the parameter but is banned by no-restricted-syntax. */
  elements.forEach((element) => {
    const { scrollLeft, scrollTop } = element;
    const previousDisplay = element.style.display;

    // The synchronous reflow drops the compositor layer. The browser does not
    // paint between hiding and restoring the element, so this is not visible.
    element.style.display = 'none';
    element.getBoundingClientRect();
    element.style.display = previousDisplay;

    element.scrollLeft = scrollLeft;
    element.scrollTop = scrollTop;
  });
  /* eslint-enable no-param-reassign */

  // Toggling the theme must not drop a keyboard user back to <body>.
  if (
    previouslyFocused instanceof HTMLElement &&
    previouslyFocused.isConnected &&
    document.activeElement !== previouslyFocused
  ) {
    previouslyFocused.focus({ preventScroll: true });
  }
}
