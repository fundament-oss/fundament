import { afterNextRender, Directive, ElementRef, inject, input } from '@angular/core';
import isPresenting from './presentation/presenting';

/**
 * Design-system overlays that focus their own `[autofocus]` element every time they
 * open. Their fields keep the real input in shadow DOM, which the browser's native
 * autofocus skips, so the overlay does it by hand.
 */
const OVERLAY_SELECTOR = 'nldd-sheet, nldd-modal-dialog';

@Directive({
  selector: '[appAutofocus]',
})
export default class AutofocusDirective {
  // Accepts `appAutofocus` (no binding → empty string) or `[appAutofocus]="bool"`
  appAutofocus = input<boolean | ''>(true);

  constructor() {
    const el = inject<ElementRef<HTMLElement>>(ElementRef).nativeElement;
    afterNextRender(() => {
      if (this.appAutofocus() === false) {
        el.removeAttribute('autofocus');
        return;
      }

      // Inside an overlay there is nothing to focus yet — the dialog is still
      // closed, and it reopens more than once. Mark the element and let the
      // overlay focus it on each open; SheetSyncDirective drops the mark for the
      // opens that happen while the walkthrough is driving.
      if (el.closest(OVERLAY_SELECTOR)) {
        el.setAttribute('autofocus', '');
        return;
      }

      // Focusing an input while presenting swallows the overlay's arrow keys.
      if (isPresenting()) return;

      // setTimeout ensures Lit's async shadow DOM render has completed
      // before calling focus(), since Lit renders on microtasks and
      // afterNextRender fires before those microtasks settle.
      setTimeout(() => el.focus());
    });
  }
}
