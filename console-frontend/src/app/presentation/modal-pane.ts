// Presentation-only: the app's modals render a native <dialog> (via showModal()),
// which the browser centers on the whole viewport. While presenting, the left 40vw
// is the narration panel, so modals should be centered in the right pane instead.
//
// The <dialog> lives inside the <nldd-modal-dialog> shadow DOM, so global page CSS
// can't reach it. Instead we adopt a tiny stylesheet into each modal's shadow root
// that offsets the dialog (and its backdrop) by --fund-modal-inset-left, a custom
// property that inherits in from :root (set in styles.css only while presenting, so
// it resolves to 0 in the plain console and this is a no-op there).

const OFFSET_SHEET = new CSSStyleSheet();
OFFSET_SHEET.replaceSync(`
  dialog.modal-dialog,
  dialog.modal-dialog::backdrop {
    left: var(--fund-modal-inset-left, 0px);
    transition: left 0.5s cubic-bezier(0.22, 1, 0.36, 1);
  }
`);

// Adopt the offset sheet into a modal's shadow root. The root may not exist yet when
// the element is first added (upgrade + first render are async), so retry across a
// few frames until it appears.
function adopt(el: Element, attempts = 10): void {
  const root = (el as HTMLElement).shadowRoot;
  if (root) {
    if (!root.adoptedStyleSheets.includes(OFFSET_SHEET)) {
      root.adoptedStyleSheets = [...root.adoptedStyleSheets, OFFSET_SHEET];
    }
    return;
  }
  if (attempts <= 0) return;
  requestAnimationFrame(() => adopt(el, attempts - 1));
}

/**
 * Start centering native modal dialogs in the right (app) pane while presenting.
 * Wires up existing and future <nldd-modal-dialog> elements; safe to call once at
 * demo boot. Does nothing outside the demo build (the offset property is unset).
 */
export default function enableModalRightPane(): void {
  document.querySelectorAll('nldd-modal-dialog').forEach((el) => adopt(el));

  const observer = new MutationObserver((records) => {
    records.forEach((record) => {
      record.addedNodes.forEach((node) => {
        if (!(node instanceof Element)) return;
        if (node.localName === 'nldd-modal-dialog') adopt(node);
        node.querySelectorAll?.('nldd-modal-dialog').forEach((el) => adopt(el));
      });
    });
  });
  observer.observe(document.body, { childList: true, subtree: true });
}
