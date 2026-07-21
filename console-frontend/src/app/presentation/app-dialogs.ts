// Helpers for the app's own modal dialogs while presenting. The app opens native
// <dialog> elements with showModal(), which trap focus and make the deck inert — so
// the walkthrough closes them when it navigates, and lets deck keys through while
// one is open. The <dialog> lives in the modal component's (open) shadow root.

type ClosableDialogHost = HTMLElement & { hide?: () => void };

/** The app modals currently open in the app pane. */
function openAppDialogs(): ClosableDialogHost[] {
  return Array.from(
    document.querySelectorAll<ClosableDialogHost>('nldd-modal-dialog'),
  ).filter((el) => !!el.shadowRoot?.querySelector('dialog[open]'));
}

/** True while the app has a modal open; it makes the deck's nav buttons inert. */
export function hasOpenAppDialog(): boolean {
  return openAppDialogs().length > 0;
}

/** Close any open app modal so it doesn't block slide navigation. */
export function closeOpenAppDialogs(): void {
  openAppDialogs().forEach((el) => el.hide?.());
}
