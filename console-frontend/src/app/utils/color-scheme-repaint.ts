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
 */
/* eslint-disable no-param-reassign -- the forEach binding is a queried DOM node this helper mutates on purpose */
export default function repaintSchemeSensitiveLayers(): void {
  document.querySelectorAll<HTMLElement>('[data-scheme-repaint]').forEach((element) => {
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
}
