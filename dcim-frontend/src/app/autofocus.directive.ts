import { afterNextRender, Directive, ElementRef, inject, input } from '@angular/core';

@Directive({
  selector: '[appAutofocus]',
})
export default class AutofocusDirective {
  // Accepts `appAutofocus` (no binding → empty string) or `[appAutofocus]="bool"`
  appAutofocus = input<boolean | ''>(true);

  constructor() {
    const el = inject<ElementRef<HTMLElement>>(ElementRef);
    afterNextRender(() => {
      if (this.appAutofocus() !== false) {
        // setTimeout ensures Lit's async shadow DOM render has completed
        // before calling focus(), since Lit renders on microtasks and
        // afterNextRender fires before those microtasks settle.
        setTimeout(() => el.nativeElement.focus());
      }
    });
  }
}
