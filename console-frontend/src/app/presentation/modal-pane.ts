// Presentation-only modal handling. Two things happen here, both demo-build only:
//
// 1. While presenting, app dialogs open *non-modally* (show() instead of showModal()).
//    A native modal dialog makes everything outside it inert, which would freeze the
//    narration deck's nav buttons; a non-modal dialog inerts nothing, so the deck stays
//    interactive. The trade-off is that a non-modal dialog has no dimmed ::backdrop.
//
// 2. The <dialog> lives inside the <nldd-modal-dialog> shadow DOM, so global page CSS
//    can't reach it. We adopt a small stylesheet into each dialog's shadow root that
//    fixes the (now non-modal) dialog to the viewport and centers it in the right pane
//    via --fund-modal-inset-left — a custom property that inherits in from :root (set
//    in styles.css only while presenting, so it resolves to 0 elsewhere).

import adoptIntoShadowRoots from './shadow-styles';

const POSITION_SHEET = new CSSStyleSheet();
POSITION_SHEET.replaceSync(`
  dialog.modal-dialog {
    position: fixed;
    top: 0;
    right: 0;
    bottom: 0;
    left: var(--fund-modal-inset-left, 0px);
    margin: auto;
    z-index: 1000;
    transition: left 0.5s cubic-bezier(0.22, 1, 0.36, 1);
  }
`);

// Open dialogs non-modally while a walkthrough is active, so the deck stays clickable.
// Outside a tour (e.g. the plain console demo) dialogs open modally as normal.
function patchShowModalWhilePresenting(): void {
  const proto = HTMLDialogElement.prototype;
  const nativeShowModal = proto.showModal;
  proto.showModal = function showModalOrPlain(this: HTMLDialogElement): void {
    if (document.documentElement.classList.contains('presenting')) {
      this.show();
    } else {
      nativeShowModal.call(this);
    }
  };
}

/**
 * Set up presentation modal behavior: dialogs open non-modally while presenting and are
 * centered in the right (app) pane. Wires up existing and future <nldd-modal-dialog>
 * elements; safe to call once at demo boot. A no-op outside the demo build.
 */
export default function enableModalRightPane(): void {
  patchShowModalWhilePresenting();
  adoptIntoShadowRoots('nldd-modal-dialog', POSITION_SHEET);
}
