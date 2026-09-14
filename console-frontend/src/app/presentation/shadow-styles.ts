// Adopting presentation-only CSS into a design-system component's shadow DOM.
//
// The components keep their real markup in shadow DOM, so page CSS cannot reach
// it and the walkthrough has nowhere to put an override. An adopted stylesheet
// can: it lands in the shadow root itself, after the component's own styles, so
// an equally specific rule wins. Both callers here (modal-pane.ts, sheet-focus.ts)
// are demo-build only — nothing in the production app adopts anything.

/**
 * Adopt `sheet` into one element's shadow root.
 *
 * The root may not exist yet when the element is first seen: custom element
 * upgrade and Lit's first render are both async. Retry across a few frames
 * until it appears, then give up rather than hold a reference forever.
 */
function adopt(el: Element, sheet: CSSStyleSheet, attempts = 10): void {
  const root = (el as HTMLElement).shadowRoot;
  if (root) {
    if (!root.adoptedStyleSheets.includes(sheet)) {
      root.adoptedStyleSheets = [...root.adoptedStyleSheets, sheet];
    }
    return;
  }
  if (attempts <= 0) return;
  requestAnimationFrame(() => adopt(el, sheet, attempts - 1));
}

/**
 * Adopt `sheet` into every current and future `tagName` element under <body>.
 *
 * Overlays are created and destroyed as the walkthrough moves between slides,
 * and a sheet is portaled to <body> when its page mounts, so the ones that
 * matter mostly do not exist yet at boot.
 */
export default function adoptIntoShadowRoots(tagName: string, sheet: CSSStyleSheet): void {
  document.querySelectorAll(tagName).forEach((el) => adopt(el, sheet));

  const observer = new MutationObserver((records) => {
    records.forEach((record) => {
      record.addedNodes.forEach((node) => {
        if (!(node instanceof Element)) return;
        if (node.localName === tagName) adopt(node, sheet);
        node.querySelectorAll?.(tagName).forEach((el) => adopt(el, sheet));
      });
    });
  });
  observer.observe(document.body, { childList: true, subtree: true });
}
