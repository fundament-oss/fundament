// Helpers for the app's own modal overlays while presenting: a modal dialog and
// a sheet are both modal, both open a native <dialog> with showModal(), and both
// trap focus and make the deck inert. So the walkthrough closes them when it
// navigates, and lets deck keys through while one is open. The <dialog> lives in
// the component's (open) shadow root.
//
// A sheet counts for the same reason a dialog does: with one open the arrow keys
// land inside it and the deck stops responding. That a sheet is a panel rather
// than a dialog makes no difference to the keyboard.
const MODAL_OVERLAYS = 'nldd-modal-dialog, nldd-sheet';

type ClosableOverlay = HTMLElement & { hide?: () => void };

/** The modal overlays currently open in the app pane. */
function openModalOverlays(): ClosableOverlay[] {
  return Array.from(document.querySelectorAll<ClosableOverlay>(MODAL_OVERLAYS)).filter(
    (el) => !!el.shadowRoot?.querySelector('dialog[open]'),
  );
}

/** True while a modal overlay is open; it makes the deck's nav buttons inert. */
export function hasOpenModalOverlay(): boolean {
  return openModalOverlays().length > 0;
}

/** Close any open modal overlay so it doesn't block slide navigation. */
export function closeModalOverlays(): void {
  openModalOverlays().forEach((el) => el.hide?.());
}
