// Helpers for the app's own modal dialogs and sheets while presenting. Both open
// native <dialog> elements with showModal(), which trap focus and make the deck
// inert — so the walkthrough closes them when it navigates, and lets deck keys
// through while one is open. The <dialog> lives in the component's (open) shadow
// root.
//
// Sheets count for the same reason modals do: with one open the arrow keys land
// inside it and the deck stops responding. That the sheet is a panel rather than
// a dialog makes no difference to the keyboard.
const MODAL_HOSTS = 'nldd-modal-dialog, nldd-sheet';

type ClosableDialogHost = HTMLElement & { hide?: () => void };

/** The app modals and sheets currently open in the app pane. */
function openAppDialogs(): ClosableDialogHost[] {
  return Array.from(document.querySelectorAll<ClosableDialogHost>(MODAL_HOSTS)).filter(
    (el) => !!el.shadowRoot?.querySelector('dialog[open]'),
  );
}

/** True while the app has a modal or sheet open; it makes the deck's nav buttons inert. */
export function hasOpenAppDialog(): boolean {
  return openAppDialogs().length > 0;
}

/** Close any open app modal or sheet so it doesn't block slide navigation. */
export function closeOpenAppDialogs(): void {
  openAppDialogs().forEach((el) => el.hide?.());
}
